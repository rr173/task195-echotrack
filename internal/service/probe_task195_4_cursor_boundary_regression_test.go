package service

import (
	"context"
	"testing"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/store"
)

func TestTask195Bug04_WindowCursorIsExclusiveAndDurable(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	if _, err := arr.Register(ctx, "arr-b4", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(ctx, "batch-b4", "arr-b4", 1); err != nil { t.Fatal(err) }
	if _, _, err := batch.IngestWindow(ctx, ingest.WindowInput{BatchID: "batch-b4", ElementNo: 1, SeqNo: 3, I: []float64{1}, Q: []float64{0}, SampleRate: 48000}); err != nil { t.Fatal(err) }
	if _, err := s.UpdateBatchState(ctx, "batch-b4", "processing", 4); err != nil { t.Fatal(err) }
	next, err := s.NextCursor(ctx, "batch-b4")
	if err != nil { t.Fatal(err) }
	if next != 4 { t.Fatalf("next cursor=%d want 4", next) }
}
