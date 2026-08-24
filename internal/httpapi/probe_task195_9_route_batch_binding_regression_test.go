package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"task195-echotrack/internal/service"
	"task195-echotrack/internal/store"
)

func TestTask195Bug09_WindowUsesRouteBatchIdentity(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	ctx := context.Background()
	arr := service.NewArrayService(s)
	batch := service.NewBatchService(s, arr)
	proc := service.NewProcessService(s, arr)
	rev := service.NewReviewService(s)
	if _, err := arr.Register(ctx, "arr-b9", "array", 48000, 1); err != nil { t.Fatal(err) }
	for _, id := range []string{"batch-b9-a", "batch-b9-b"} {
		if _, err := batch.Create(ctx, id, "arr-b9", 1); err != nil { t.Fatal(err) }
	}
	srv := New(NewHandler(arr, batch, proc, rev), log.New(io.Discard, "", 0))
	req := httptest.NewRequest(http.MethodPost, "/api/batches/batch-b9-a/windows", strings.NewReader(`{"batchId":"batch-b9-b","elementNo":1,"seqNo":0,"i":[1],"q":[0],"sampleRate":48000}`))
	out := httptest.NewRecorder()
	srv.Handler().ServeHTTP(out, req)
	if out.Code != http.StatusCreated || !strings.Contains(out.Body.String(), `"id":"batch-b9-a"`) { t.Fatalf("response code=%d body=%s", out.Code, out.Body.String()) }
	windowsA, err := batch.ListWindows(ctx, "batch-b9-a")
	if err != nil { t.Fatal(err) }
	windowsB, err := batch.ListWindows(ctx, "batch-b9-b")
	if err != nil { t.Fatal(err) }
	if len(windowsA) != 1 || len(windowsB) != 0 { t.Fatalf("windows A=%d B=%d", len(windowsA), len(windowsB)) }
}
