package service

import (
	"context"
	"testing"

	"task195-echotrack/internal/model"
	"task195-echotrack/internal/review"
	"task195-echotrack/internal/store"
)

func TestTask195Bug06_SupersedeUsesBatchScopedVersion(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	rev := NewReviewService(s)
	if _, err := arr.Register(ctx, "arr-b6", "array", 48000, 1); err != nil { t.Fatal(err) }
	for _, id := range []string{"batch-b6-a", "batch-b6-b"} {
		if _, err := batch.Create(ctx, id, "arr-b6", 1); err != nil { t.Fatal(err) }
		if _, err := s.UpdateBatchState(ctx, id, model.BatchStatusProcessing, 0); err != nil { t.Fatal(err) }
		if _, err := s.UpdateBatchState(ctx, id, model.BatchStatusReviewing, 0); err != nil { t.Fatal(err) }
		if err := s.CreateInterpretation(ctx, &model.Interpretation{ID: id + "-intp", BatchID: id, Version: 1, Status: model.InterpretationStatusPublished, Title: "v1", SnapshotRef: id + "@v1", CreatedAt: Now()}); err != nil { t.Fatal(err) }
	}
	next, err := rev.SupersedeInterpretation(ctx, review.SupersedeRequest{BatchID: "batch-b6-a", OldVersion: 1, NewTitle: "v2", Author: "qa"})
	if err != nil { t.Fatal(err) }
	if next.BatchID != "batch-b6-a" { t.Fatalf("superseded batch=%s", next.BatchID) }
}
