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

func TestTask195Bug02_CancelledUploadDoesNotCommit(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	arr := service.NewArrayService(s)
	batch := service.NewBatchService(s, arr)
	proc := service.NewProcessService(s, arr)
	rev := service.NewReviewService(s)
	if _, err := arr.Register(context.Background(), "arr-b2", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(context.Background(), "batch-b2", "arr-b2", 1); err != nil { t.Fatal(err) }
	srv := New(NewHandler(arr, batch, proc, rev), log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/batches/batch-b2/windows", strings.NewReader(`{"elementNo":1,"seqNo":0,"i":[1],"q":[0],"sampleRate":48000}`))
	out := httptest.NewRecorder()
	srv.Handler().ServeHTTP(out, req)
	if out.Code == http.StatusCreated { t.Fatalf("cancelled upload committed: %s", out.Body.String()) }
	windows, err := batch.ListWindows(context.Background(), "batch-b2")
	if err != nil { t.Fatal(err) }
	if len(windows) != 0 { t.Fatalf("cancelled upload left %d windows", len(windows)) }
}
