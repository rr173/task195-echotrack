package service

import (
	"context"
	"testing"

	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/store"
)

func TestTask195Bug08_ProcessPreservesIngestCursor(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	proc := NewProcessService(s, arr)
	if _, err := arr.Register(ctx, "arr-b8", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(ctx, "batch-b8", "arr-b8", 1); err != nil { t.Fatal(err) }
	if _, _, err := batch.IngestWindow(ctx, ingest.WindowInput{BatchID: "batch-b8", ElementNo: 1, SeqNo: 2, I: []float64{1, 0}, Q: []float64{0, 1}, SampleRate: 48000}); err != nil { t.Fatal(err) }
	if _, err := s.UpdateBatchState(ctx, "batch-b8", "processing", 3); err != nil { t.Fatal(err) }
	before, err := batch.Get(ctx, "batch-b8")
	if err != nil { t.Fatal(err) }
	if _, err := proc.Process(ctx, "batch-b8", fitting.DefaultOptions()); err != nil { t.Fatal(err) }
	after, err := batch.Get(ctx, "batch-b8")
	if err != nil { t.Fatal(err) }
	if after.WindowCursor != before.WindowCursor { t.Fatalf("cursor changed from %d to %d", before.WindowCursor, after.WindowCursor) }
}
