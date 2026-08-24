package store

import (
	"context"
	"testing"
	"time"

	"task195-echotrack/internal/model"
)

func TestOpenPersistsArrayAcrossRestart(t *testing.T) {
	dbPath := t.TempDir() + "/echotrack.db"
	ctx := context.Background()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := model.NewArray("arr-store-test", "store test", 48000, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateArray(ctx, arr); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	recovered, err := s.GetArray(ctx, arr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Name != arr.Name || len(recovered.Elements) != 2 {
		t.Fatalf("recovered array mismatch: %+v", recovered)
	}
}

// TestBatchCursorPersistsExclusiveUpperBound 验证上传序号 3 的窗口后，
// 持久化游标为排他上界 4（下一个待处理序号），并在重启后保持不变。
func TestBatchCursorPersistsExclusiveUpperBound(t *testing.T) {
	dbPath := t.TempDir() + "/echotrack.db"
	ctx := context.Background()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	arr, err := model.NewArray("arr-cursor-test", "cursor test", 48000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateArray(ctx, arr); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	batch := &model.Batch{
		ID:               "batch-cursor",
		ArrayID:          "arr-cursor-test",
		Status:           model.BatchStatusUploading,
		ReferenceElement: 1,
		WindowCursor:      0,
		SampleRateHz:     48000,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.CreateBatch(ctx, batch); err != nil {
		t.Fatal(err)
	}

	// 上传序号 3 的第一个窗口：游标应推进到排他上界 4。
	win := &model.Window{
		ID:         "win-3",
		BatchID:    "batch-cursor",
		ElementNo:  1,
		SeqNo:      3,
		I:          []float64{1, 2},
		Q:          []float64{1, 2},
		SampleRate: 48000,
		Status:     model.WindowStatusNew,
		Checksum:   "abc",
		CreatedAt:  now,
	}
	if _, inserted, err := s.UpsertWindow(ctx, win); err != nil || !inserted {
		t.Fatalf("upsert window: inserted=%v err=%v", inserted, err)
	}
	nextCursor := model.AdvanceWindowCursor(batch.WindowCursor, 3)
	if nextCursor != 4 {
		t.Fatalf("AdvanceWindowCursor(0,3)=%d want 4", nextCursor)
	}
	// 进入 processing 是合法流转，同时把游标推进到排他上界 4。
	if _, err := s.UpdateBatchState(ctx, batch.ID, model.BatchStatusProcessing, nextCursor); err != nil {
		t.Fatal(err)
	}

	// 从持久化存储读取：游标仍是排他上界 4。
	got, err := s.NextCursor(ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Errorf("NextCursor before restart=%d want 4 (exclusive upper bound)", got)
	}
	b1, err := s.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b1.WindowCursor != 4 {
		t.Errorf("WindowCursor before restart=%d want 4", b1.WindowCursor)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// 重启后从持久化存储恢复：排他上界保持不变。
	s2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got2, err := s2.NextCursor(ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != 4 {
		t.Errorf("NextCursor after restart=%d want 4 (exclusive upper bound preserved)", got2)
	}
	b2, err := s2.GetBatch(ctx, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b2.WindowCursor != 4 {
		t.Errorf("WindowCursor after restart=%d want 4 (exclusive upper bound preserved)", b2.WindowCursor)
	}
}
