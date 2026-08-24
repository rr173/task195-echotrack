package service

import (
	"context"
	"sync"
	"testing"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/store"
)

func TestTask195Bug07_ConcurrentDuplicateWindowHasOneInsert(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	if _, err := arr.Register(ctx, "arr-b7", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(ctx, "batch-b7", "arr-b7", 1); err != nil { t.Fatal(err) }
	start := make(chan struct{})
	type result struct { inserted, duplicated bool; err error }
	results := make(chan result, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, receipt, err := batch.IngestWindow(ctx, ingest.WindowInput{BatchID: "batch-b7", ElementNo: 1, SeqNo: 0, I: []float64{1, 2}, Q: []float64{0, 0}, SampleRate: 48000})
			if receipt != nil { results <- result{receipt.Inserted, receipt.Duplicated, err} } else { results <- result{err: err} }
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	inserted, duplicated := 0, 0
	for r := range results {
		if r.err != nil { t.Fatal(r.err) }
		if r.inserted { inserted++ }
		if r.duplicated { duplicated++ }
	}
	if inserted != 1 || duplicated != 19 { t.Fatalf("inserted=%d duplicated=%d", inserted, duplicated) }
	windows, err := batch.ListWindows(ctx, "batch-b7")
	if err != nil { t.Fatal(err) }
	if len(windows) != 1 { t.Fatalf("stored windows=%d", len(windows)) }
}
