package service

import (
	"context"
	"testing"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/model"
	"task195-echotrack/internal/store"
)

func TestTask195Bug05_PublishedBatchRejectsMutation(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	rev := NewReviewService(s)
	if _, err := arr.Register(ctx, "arr-b5", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(ctx, "batch-b5", "arr-b5", 1); err != nil { t.Fatal(err) }
	if _, err := s.UpdateBatchState(ctx, "batch-b5", model.BatchStatusProcessing, 0); err != nil { t.Fatal(err) }
	if _, err := s.UpdateBatchState(ctx, "batch-b5", model.BatchStatusReviewing, 0); err != nil { t.Fatal(err) }
	in, err := rev.CreateInterpretation(ctx, "batch-b5", "freeze")
	if err != nil { t.Fatal(err) }
	if _, err := rev.PublishInterpretation(ctx, in.ID); err != nil { t.Fatal(err) }
	if _, _, err := batch.IngestWindow(ctx, ingest.WindowInput{BatchID: "batch-b5", ElementNo: 1, SeqNo: 0, I: []float64{1}, Q: []float64{0}, SampleRate: 48000}); err == nil { t.Fatal("published batch accepted a window") }
	if _, err := batch.ChangeReferenceElement(ctx, "batch-b5", 1); err == nil { t.Fatal("published batch accepted reference change") }
}
