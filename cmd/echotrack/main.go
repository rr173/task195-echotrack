// echotrack 声学阵列回波相位追踪台。
// 入口契约：
//   --addr :8080        HTTP 监听地址
//   --db <path>         SQLite 数据库路径（默认 echotrack.db）
//   --smoke-test        自检模式：创建数据 -> 执行核心闭环 -> 关闭重开数据库
//                       验证持久化与重启恢复，全程以 0 退出码结束。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/httpapi"
	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/review"
	"task195-echotrack/internal/service"
	"task195-echotrack/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "http listen address")
	dbPath := flag.String("db", "echotrack.db", "sqlite database path")
	smoke := flag.Bool("smoke-test", false, "run end-to-end self test and exit")
	flag.Parse()

	logger := log.New(os.Stdout, "[echotrack] ", log.LstdFlags)

	if *smoke {
		if err := runSmokeTest(*dbPath, logger); err != nil {
			logger.Printf("SMOKE TEST FAILED: %v", err)
			os.Exit(1)
		}
		logger.Printf("SMOKE TEST PASSED")
		return
	}

	s, err := store.Open(*dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer s.Close()

	h := buildHandler(s, logger)
	server := httpapi.New(h, logger)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Printf("listening on %s (db=%s)", *addr, *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("serve: %v", err)
	}
}

// buildHandler 组装领域服务并构造 HTTP Handler。
func buildHandler(s *store.Store, logger *log.Logger) *httpapi.Handler {
	arrSvc := service.NewArrayService(s)
	batchSvc := service.NewBatchService(s, arrSvc)
	procSvc := service.NewProcessService(s, arrSvc)
	revSvc := service.NewReviewService(s)
	_ = logger
	return httpapi.NewHandler(arrSvc, batchSvc, procSvc, revSvc)
}

// runSmokeTest 端到端自检：
//  1. 打开数据库，注册四阵元阵列；
//  2. 创建批次并上传四阵元采样窗口（含一个相位跳变）；
//  3. 执行处理流水线，断言轨迹生成、断裂检测；
//  4. 标注区段、切换参考阵元重算、确认轨迹；
//  5. 创建并发布解释包；
//  6. 关闭并重新打开同一数据库，验证状态恢复与幂等。
func runSmokeTest(dbPath string, logger *log.Logger) error {
	// 清理历史自检数据库，保证可重复运行。
	_ = os.Remove(dbPath)

	s, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	ctx := context.Background()

	arrSvc := service.NewArrayService(s)
	batchSvc := service.NewBatchService(s, arrSvc)
	procSvc := service.NewProcessService(s, arrSvc)
	revSvc := service.NewReviewService(s)

	// 1. 注册四阵元阵列。
	arr, err := arrSvc.Register(ctx, "arr-demo", "demo-array-4ch", 48000, 4)
	if err != nil {
		return fmt.Errorf("register array: %w", err)
	}
	logger.Printf("registered array %s (%d elements, %.0f Hz)", arr.ID, len(arr.Elements), arr.SampleRateHz)

	// 2. 创建批次。
	batch, err := batchSvc.Create(ctx, "batch-demo", "arr-demo", 1)
	if err != nil {
		return fmt.Errorf("create batch: %w", err)
	}
	logger.Printf("created batch %s status=%s", batch.ID, batch.Status)

	// 构造复采样窗口：相位线性 + 一个跳变。
	for el := 1; el <= 4; el++ {
		for seq := int64(0); seq < 3; seq++ {
			var i, q []float64
			for k := 0; k < 64; k++ {
				t := float64(seq*64+int64(k)) / batch.SampleRateHz
				phase := 2*3.141592653589793*1000*t + float64(el)*0.1
				if el == 2 && seq == 1 && k >= 32 {
					phase += 2.0 // 阵元2中段注入相位跳变
				}
				i = append(i, math.Cos(phase))
				q = append(q, math.Sin(phase))
			}
			receipt, err := ingestWindow(batchSvc, ctx, "batch-demo", el, seq, i, q, batch.SampleRateHz)
			if err != nil {
				return err
			}
			if receipt != nil {
				logger.Printf("window el=%d seq=%d checksum=%s inserted=%v", el, seq, receipt.Checksum, receipt.Inserted)
			}
		}
	}

	// 3. 处理流水线。
	res, err := procSvc.Process(ctx, "batch-demo", fitting.DefaultOptions())
	if err != nil {
		return fmt.Errorf("process: %w", err)
	}
	logger.Printf("processed batch: status=%s corrected=%d jumps=%d tracks=%d",
		res.Status, res.Corrected, res.JumpCount, len(res.Tracks))
	if res.Status != "reviewing" {
		return fmt.Errorf("expected batch reviewing, got %s", res.Status)
	}
	if len(res.Tracks) == 0 {
		return errors.New("expected at least one track")
	}

	// 4. 标注 + 切换参考阵元重算 + 确认轨迹。
	_, err = revSvc.AddAnnotation(ctx, review.AnnotationInput{
		BatchID: "batch-demo", TargetType: "track", TargetID: res.Tracks[0],
		Note: "中段疑似遮挡区段", Author: "smoke",
	})
	if err != nil {
		return fmt.Errorf("add annotation: %w", err)
	}
	track, err := procSvc.Track(ctx, res.Tracks[0])
	if err != nil {
		return fmt.Errorf("get track: %w", err)
	}
	if len(track.PhasePoints) == 0 {
		return errors.New("track has no phase points")
	}
	_, err = revSvc.AddSegment(ctx, review.SegmentDecision{
		TrackID: track.ID, StartIdx: 0, EndIdx: len(track.PhasePoints) - 1,
		Label: "回波主路径", Author: "smoke", Confirmed: true,
	})
	if err != nil {
		return fmt.Errorf("add segment: %w", err)
	}
	// 切换参考阵元到 3 并重算。
	_, err = procSvc.DifferentialRecompute(ctx, "batch-demo", 3)
	if err != nil {
		return fmt.Errorf("recompute: %w", err)
	}

	// 5. 创建并发布解释包。
	intp, err := revSvc.CreateInterpretation(ctx, "batch-demo", "demo-batch-解释包-v1")
	if err != nil {
		return fmt.Errorf("create interpretation: %w", err)
	}
	published, err := revSvc.PublishInterpretation(ctx, intp.ID)
	if err != nil {
		return fmt.Errorf("publish interpretation: %w", err)
	}
	if published.Status != "published" {
		return fmt.Errorf("expected published, got %s", published.Status)
	}
	logger.Printf("published interpretation %s snapshot=%s tracks=%d", published.ID, published.SnapshotRef, published.TrackCount)

	// 6. 关闭并重开同一数据库，验证恢复。
	if err := s.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	s2, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("reopen: %w", err)
	}
	defer s2.Close()

	batch2, err := s2.GetBatch(ctx, "batch-demo")
	if err != nil {
		return fmt.Errorf("reopen get batch: %w", err)
	}
	if batch2.Status != "published" {
		return fmt.Errorf("expected published after reopen, got %s", batch2.Status)
	}
	tracks2, err := s2.ListTracks(ctx, "batch-demo")
	if err != nil {
		return fmt.Errorf("reopen list tracks: %w", err)
	}
	annots2, err := s2.ListAnnotations(ctx, "batch-demo")
	if err != nil {
		return fmt.Errorf("reopen list annotations: %w", err)
	}
	intps2, err := s2.ListInterpretations(ctx, "batch-demo")
	if err != nil {
		return fmt.Errorf("reopen list interpretations: %w", err)
	}
	logger.Printf("recovered after reopen: batch=%s tracks=%d annotations=%d interpretations=%d",
		batch2.Status, len(tracks2), len(annots2), len(intps2))
	if len(tracks2) == 0 || len(annots2) == 0 || len(intps2) != 1 {
		return errors.New("recovery incomplete: tracks/annotations/interpretations missing")
	}
	return nil
}

// ingestWindow 上传窗口并返回回执。
func ingestWindow(b *service.BatchService, ctx context.Context, batchID string, el int, seq int64, i, q []float64, rate float64) (*ingest.IngestReceipt, error) {
	_, receipt, err := b.IngestWindow(ctx, ingest.WindowInput{
		BatchID: batchID, ElementNo: el, SeqNo: seq, I: i, Q: q, SampleRate: rate,
	})
	if err != nil {
		return nil, fmt.Errorf("ingest window el=%d seq=%d: %w", el, seq, err)
	}
	return receipt, nil
}
