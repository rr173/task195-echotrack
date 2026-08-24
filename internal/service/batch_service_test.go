package service

import (
	"context"
	"sync"
	"testing"

	"task195-echotrack/internal/ingest"
	"task195-echotrack/internal/store"
)

// TestIngestWindowConcurrentSameWindowIdempotent 验证并发提交同一窗口的幂等性：
// 20 个请求同时上传同一个 (batchId, elementNo, seqNo) 窗口时，只能有一个插入成功，
// 其余请求得到幂等重复回执，数据库中最终只保留一条窗口记录。
func TestIngestWindowConcurrentSameWindowIdempotent(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	arr := NewArrayService(s)
	batch := NewBatchService(s, arr)
	ctx := context.Background()

	arrayID := "arr-conc"
	if _, err := arr.Register(ctx, arrayID, "concurrent test", 48000, 2); err != nil {
		t.Fatal(err)
	}
	batchID := "batch-conc"
	if _, err := batch.Create(ctx, batchID, arrayID, 1); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	type result struct {
		receipt *ingest.IngestReceipt
		err     error
	}
	results := make([]result, n)
	// 同一窗口内容：20 个并发请求完全相同。
	in := ingest.WindowInput{
		BatchID: batchID, ElementNo: 1, SeqNo: 0,
		I: []float64{0.1, 0.2, 0.3, 0.4}, Q: []float64{1, 2, 3, 4},
		SampleRate: 48000,
	}
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			_, receipt, err := batch.IngestWindow(ctx, in)
			results[i] = result{receipt: receipt, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	inserted := 0
	duplicated := 0
	errored := 0
	for i, r := range results {
		if r.err != nil {
			errored++
			t.Errorf("request %d errored: %v", i, r.err)
			continue
		}
		if r.receipt == nil {
			t.Errorf("request %d: nil receipt", i)
			continue
		}
		if r.receipt.Inserted {
			inserted++
		} else if r.receipt.Duplicated {
			duplicated++
		} else {
			t.Errorf("request %d: receipt neither inserted nor duplicated: %+v", i, r.receipt)
		}
	}
	if inserted != 1 {
		t.Errorf("expected exactly 1 insertion, got %d", inserted)
	}
	if duplicated != n-1 {
		t.Errorf("expected %d idempotent duplicates, got %d", n-1, duplicated)
	}
	if errored != 0 {
		t.Errorf("expected 0 errors, got %d", errored)
	}

	// 数据库中最终只能保留一条该窗口记录。
	wins, err := batch.ListWindows(ctx, batchID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wins) != 1 {
		t.Fatalf("expected 1 window row persisted, got %d", len(wins))
	}
	wantChecksum := ingest.Checksum(in.I, in.Q)
	if wins[0].Checksum != wantChecksum {
		t.Errorf("persisted window checksum mismatch: got %s want %s", wins[0].Checksum, wantChecksum)
	}
}
