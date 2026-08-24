package service

import (
	"context"
	"math"
	"testing"

	"task195-echotrack/internal/fitting"
	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/store"
)

func TestTask195Bug03_ProcessKeepsWindowSequenceOrder(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batchSvc := NewBatchService(s, arr)
	proc := NewProcessService(s, arr)
	if _, err := arr.Register(ctx, "arr-b3", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batchSvc.Create(ctx, "batch-b3", "arr-b3", 1); err != nil { t.Fatal(err) }
	for seq, phase := range []float64{0.1, 1.1} {
		if _, _, err := batchSvc.IngestWindow(ctx, ingest.WindowInput{BatchID: "batch-b3", ElementNo: 1, SeqNo: int64(seq), I: []float64{math.Cos(phase)}, Q: []float64{math.Sin(phase)}, SampleRate: 48000}); err != nil { t.Fatal(err) }
	}
	if _, err := proc.Process(ctx, "batch-b3", fitting.DefaultOptions()); err != nil { t.Fatal(err) }
	tracks, err := proc.Tracks(ctx, "batch-b3")
	if err != nil { t.Fatal(err) }
	if len(tracks) != 1 || len(tracks[0].PhasePoints) != 2 { t.Fatalf("tracks=%+v", tracks) }
	if tracks[0].PhasePoints[0] >= tracks[0].PhasePoints[1] || tracks[0].Status != "continuous" {
		t.Fatalf("sequence order lost: %+v", tracks[0])
	}
}
