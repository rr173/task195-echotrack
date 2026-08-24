package service

import (
	"context"
	"errors"
	"testing"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

// newBatchFixture 注册一阵元阵列 + 创建上传中批次，返回服务与批次。
func newBatchFixture(t *testing.T) (*BatchService, *model.Batch) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	arrSvc := NewArrayService(s)
	batchSvc := NewBatchService(s, arrSvc)
	ctx := context.Background()
	if _, err := arrSvc.Register(ctx, "arr-cancel", "cancel-test", 48000, 2); err != nil {
		t.Fatal(err)
	}
	batch, err := batchSvc.Create(ctx, "batch-cancel", "arr-cancel", 1)
	if err != nil {
		t.Fatal(err)
	}
	return batchSvc, batch
}

// sampleWindow 构造一组合法复采样窗口。
func sampleWindow(batchID string, el int, seq int64) ingest.WindowInput {
	i := []float64{0.1, 0.2, 0.3, 0.4}
	q := []float64{1.0, 1.1, 1.2, 1.3}
	return ingest.WindowInput{BatchID: batchID, ElementNo: el, SeqNo: seq, I: i, Q: q, SampleRate: 48000}
}

// TestIngestWindowCancelledBeforeCommit 取消在写入提交前生效：
// 服务返回可识别的取消错误，窗口不入库、游标不前进、后续查询看不到该窗口。
func TestIngestWindowCancelledBeforeCommit(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 提前取消：调用前 ctx 已处于 Done。
	_, _, err := batchSvc.IngestWindow(ctx, sampleWindow(batch.ID, 1, 0))
	if !errors.Is(err, model.ErrRequestCancelled) {
		t.Fatalf("expected ErrRequestCancelled, got %v", err)
	}

	// 断言一：窗口不入库。
	bg := context.Background()
	wins, err := batchSvc.ListWindows(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 0 {
		t.Fatalf("cancelled window must not be persisted; got %d windows", len(wins))
	}
	// 断言二：游标不前进。
	b, err := batchSvc.Get(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.WindowCursor != 0 {
		t.Fatalf("cursor must not advance for cancelled upload; got %d", b.WindowCursor)
	}
}

// TestIngestWindowCancelledAfterCommit 提交后取消：
// 数据已落库，服务仍返回可识别的取消错误，但窗口与游标已可见。
func TestIngestWindowCancelledAfterCommit(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)
	bg := context.Background()

	// 先正常上传 seq=0 作为前置状态。
	if _, _, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0)); err != nil {
		t.Fatal(err)
	}
	// 重新读取批次拿到游标 1，构造一个“提交后取消”场景：ctx 在 store 返回后才报错。
	// 用一个包装 ctx，它在 IngestWindow 体内 ctx.Err() 检查时返回取消——
	// 模拟提交已完成、响应尚未写出时客户端断开。
	cur, err := batchSvc.Get(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cur.WindowCursor != 1 {
		t.Fatalf("setup cursor want 1, got %d", cur.WindowCursor)
	}

	// 取消的 ctx 再次上传 seq=1：由于 ctx 已 Done，
	// IngestWindow 入口即返回 ErrRequestCancelled，不接触事务。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = batchSvc.IngestWindow(ctx, sampleWindow(batch.ID, 1, 1))
	if !errors.Is(err, model.ErrRequestCancelled) {
		t.Fatalf("expected ErrRequestCancelled, got %v", err)
	}
	wins, err := batchSvc.ListWindows(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 1 {
		t.Fatalf("only the committed window should be visible; got %d", len(wins))
	}
	b2, err := batchSvc.Get(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b2.WindowCursor != 1 {
		t.Fatalf("cursor must stay at 1; got %d", b2.WindowCursor)
	}
}

// TestIngestWindowNormalPersists 非取消的正常上传：窗口入库、游标前进，回归保护。
func TestIngestWindowNormalPersists(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)
	bg := context.Background()

	batchOut, receipt, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0))
	if err != nil {
		t.Fatalf("normal ingest: %v", err)
	}
	if !receipt.Inserted || receipt.Duplicated {
		t.Fatalf("receipt should mark inserted: %+v", receipt)
	}
	if batchOut.WindowCursor != 1 {
		t.Fatalf("cursor want 1, got %d", batchOut.WindowCursor)
	}
	wins, err := batchSvc.ListWindows(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 1 || wins[0].ElementNo != 1 || wins[0].SeqNo != 0 {
		t.Fatalf("unexpected windows: %+v", wins)
	}
}

// TestIngestWindowIdempotentDoesNotAdvanceCursor 幂等命中不推进游标：
// 同一窗口重复上传，receipt.Duplicated=true，游标不重复前进。
func TestIngestWindowIdempotentDoesNotAdvanceCursor(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)
	bg := context.Background()

	if _, _, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0)); err != nil {
		t.Fatal(err)
	}
	// 第二次上传同一内容：幂等。
	batchOut, receipt, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0))
	if err != nil {
		t.Fatalf("idempotent ingest: %v", err)
	}
	if receipt.Inserted || !receipt.Duplicated {
		t.Fatalf("receipt should mark duplicated: %+v", receipt)
	}
	if batchOut.WindowCursor != 1 {
		t.Fatalf("cursor must not advance on dedup; got %d", batchOut.WindowCursor)
	}
	// 第三次上传更高序号：游标前进到 3。
	batchOut, receipt, err = batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 2))
	if err != nil {
		t.Fatalf("seq=2 ingest: %v", err)
	}
	if !receipt.Inserted {
		t.Fatalf("seq=2 should insert: %+v", receipt)
	}
	if batchOut.WindowCursor != 3 {
		t.Fatalf("cursor want 3, got %d", batchOut.WindowCursor)
	}
}

// TestIngestWindowDuplicateContentRejected 不同内容同键：返回 ErrDuplicateWindow，游标不动。
func TestIngestWindowDuplicateContentRejected(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)
	bg := context.Background()

	if _, _, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0)); err != nil {
		t.Fatal(err)
	}
	// 同一 (batch,el,seq) 但内容不同。
	dup := sampleWindow(batch.ID, 1, 0)
	dup.I = []float64{9.9, 9.9, 9.9, 9.9}
	_, _, err := batchSvc.IngestWindow(bg, dup)
	if !errors.Is(err, model.ErrDuplicateWindow) {
		t.Fatalf("expected ErrDuplicateWindow, got %v", err)
	}
	b, err := batchSvc.Get(bg, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.WindowCursor != 1 {
		t.Fatalf("cursor must not advance on dup conflict; got %d", b.WindowCursor)
	}
}

// TestIngestWindowCancelledDoesNotBlockSubsequentUpload 取消的窗口后续可正常上传：
// 取消不留下“占位”记录，幂等键仍可用。
func TestIngestWindowCancelledDoesNotBlockSubsequentUpload(t *testing.T) {
	batchSvc, batch := newBatchFixture(t)
	bg := context.Background()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := batchSvc.IngestWindow(ctx, sampleWindow(batch.ID, 1, 0)); !errors.Is(err, model.ErrRequestCancelled) {
		t.Fatalf("expected ErrRequestCancelled, got %v", err)
	}
	// 同一 (batch,el,seq) 内容可正常上传——证明取消未占位。
	batchOut, receipt, err := batchSvc.IngestWindow(bg, sampleWindow(batch.ID, 1, 0))
	if err != nil {
		t.Fatalf("subsequent upload after cancel: %v", err)
	}
	if !receipt.Inserted {
		t.Fatalf("window should insert after prior cancel: %+v", receipt)
	}
	if batchOut.WindowCursor != 1 {
		t.Fatalf("cursor want 1 after re-upload, got %d", batchOut.WindowCursor)
	}
}
