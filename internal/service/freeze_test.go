package service

import (
	"context"
	"errors"
	"testing"

	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// newFrozenBatchFixture 注册阵列、创建批次、上传并处理到一个 reviewing 批次。
func newFrozenBatchFixture(t *testing.T) (*store.Store, *ArrayService, *BatchService, *ProcessService, *ReviewService, *model.Batch) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	arrSvc := NewArrayService(s)
	batchSvc := NewBatchService(s, arrSvc)
	procSvc := NewProcessService(s, arrSvc)
	revSvc := NewReviewService(s)

	if _, err := arrSvc.Register(ctx, "arr", "arr", 48000, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := batchSvc.Create(ctx, "b", "arr", 1); err != nil {
		t.Fatal(err)
	}
	for el := 1; el <= 4; el++ {
		for seq := int64(0); seq < 2; seq++ {
			i := []float64{float64(seq), float64(el)}
			q := []float64{0, 0.1}
			if _, _, err := batchSvc.IngestWindow(ctx, ingest.WindowInput{
				BatchID: "b", ElementNo: el, SeqNo: seq, I: i, Q: q, SampleRate: 48000,
			}); err != nil {
				t.Fatalf("ingest el=%d seq=%d: %v", el, seq, err)
			}
		}
	}
	res, err := procSvc.Process(ctx, "b", fitting.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks) == 0 {
		t.Fatal("expected tracks")
	}
	batch, err := batchSvc.Get(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	return s, arrSvc, batchSvc, procSvc, revSvc, batch
}

func TestPublishedBatchRejectsWindowIngest(t *testing.T) {
	ctx := context.Background()
	_, _, batchSvc, _, revSvc, _ := newFrozenBatchFixture(t)

	intp, err := revSvc.CreateInterpretation(ctx, "b", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revSvc.PublishInterpretation(ctx, intp.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	batch, _ := batchSvc.Get(ctx, "b")
	if batch.Status != model.BatchStatusPublished {
		t.Fatalf("batch status=%s want published", batch.Status)
	}

	// 发布后迟到窗口必须被拒绝，旧包快照不变。
	i := []float64{9}
	q := []float64{9}
	_, _, err = batchSvc.IngestWindow(ctx, ingest.WindowInput{
		BatchID: "b", ElementNo: 1, SeqNo: 99, I: i, Q: q, SampleRate: 48000,
	})
	if !errors.Is(err, model.ErrFrozenBatch) {
		t.Fatalf("expected ErrFrozenBatch, got %v", err)
	}
}

func TestPublishedBatchRejectsReferenceSwitch(t *testing.T) {
	ctx := context.Background()
	s, _, batchSvc, procSvc, revSvc, _ := newFrozenBatchFixture(t)

	intp, err := revSvc.CreateInterpretation(ctx, "b", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revSvc.PublishInterpretation(ctx, intp.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 直接切换参考阵元：冻结。
	if _, err := batchSvc.ChangeReferenceElement(ctx, "b", 3); !errors.Is(err, model.ErrFrozenBatch) {
		t.Fatalf("change reference: expected ErrFrozenBatch, got %v", err)
	}
	// 经由重算切换参考阵元：冻结。
	if _, err := procSvc.DifferentialRecompute(ctx, "b", 3); !errors.Is(err, model.ErrFrozenBatch) {
		t.Fatalf("recompute: expected ErrFrozenBatch, got %v", err)
	}
	// 参考阵元与游标未变。
	batch, _ := s.GetBatch(ctx, "b")
	if batch.ReferenceElement != 1 {
		t.Errorf("reference=%d want 1 (frozen)", batch.ReferenceElement)
	}
}

func TestPublishedBatchRejectsIgnoreWindow(t *testing.T) {
	ctx := context.Background()
	s, _, batchSvc, _, revSvc, _ := newFrozenBatchFixture(t)

	intp, err := revSvc.CreateInterpretation(ctx, "b", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revSvc.PublishInterpretation(ctx, intp.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 发布后忽略窗口会改动快照内窗口状态，必须拒绝。
	if err := batchSvc.IgnoreWindow(ctx, "b", 1, 0); !errors.Is(err, model.ErrFrozenBatch) {
		t.Fatalf("ignore window: expected ErrFrozenBatch, got %v", err)
	}
	wins, err := s.ListWindows(ctx, "b")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range wins {
		if w.ElementNo == 1 && w.SeqNo == 0 && w.Status == model.WindowStatusIgnored {
			t.Errorf("window el=1 seq=0 should remain in snapshot, got ignored")
		}
	}
}

func TestPublishedBatchPreservesSnapshot(t *testing.T) {
	ctx := context.Background()
	s, _, batchSvc, _, revSvc, _ := newFrozenBatchFixture(t)

	// 快照基线：发布前窗口数与轨迹数。
	winsBefore, _ := s.ListWindows(ctx, "b")
	tracksBefore, _ := s.ListTracks(ctx, "b")

	intp, err := revSvc.CreateInterpretation(ctx, "b", "v1")
	if err != nil {
		t.Fatal(err)
	}
	published, err := revSvc.PublishInterpretation(ctx, intp.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 尝试一系列发布后改动：均应被拒。
	_, _, _ = batchSvc.IngestWindow(ctx, ingest.WindowInput{
		BatchID: "b", ElementNo: 1, SeqNo: 50, I: []float64{1}, Q: []float64{1}, SampleRate: 48000,
	})
	_ = batchSvc.IgnoreWindow(ctx, "b", 1, 0)

	// 快照保持不变：窗口数、轨迹数、窗口状态、批次状态均未变。
	winsAfter, _ := s.ListWindows(ctx, "b")
	tracksAfter, _ := s.ListTracks(ctx, "b")
	if len(winsAfter) != len(winsBefore) {
		t.Errorf("window count changed: %d -> %d", len(winsBefore), len(winsAfter))
	}
	if len(tracksAfter) != len(tracksBefore) {
		t.Errorf("track count changed: %d -> %d", len(tracksBefore), len(tracksAfter))
	}
	batch, _ := batchSvc.Get(ctx, "b")
	if batch.Status != model.BatchStatusPublished {
		t.Errorf("batch status=%s want published", batch.Status)
	}
	// 解释包状态保持 published。
	got, _ := s.GetInterpretation(ctx, published.ID)
	if got.Status != model.InterpretationStatusPublished {
		t.Errorf("interpretation status=%s want published", got.Status)
	}
}
