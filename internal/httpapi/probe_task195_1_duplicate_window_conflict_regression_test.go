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

func TestTask195Bug01_DuplicateWindowConflictPreservesStatus(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil { t.Fatal(err) }
	defer s.Close()
	arr := service.NewArrayService(s)
	batch := service.NewBatchService(s, arr)
	proc := service.NewProcessService(s, arr)
	rev := service.NewReviewService(s)
	if _, err := arr.Register(context.Background(), "arr-b1", "array", 48000, 1); err != nil { t.Fatal(err) }
	if _, err := batch.Create(context.Background(), "batch-b1", "arr-b1", 1); err != nil { t.Fatal(err) }
	srv := New(NewHandler(arr, batch, proc, rev), log.New(io.Discard, "", 0))
	post := func(payload string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/batches/batch-b1/windows", strings.NewReader(payload))
		out := httptest.NewRecorder()
		srv.Handler().ServeHTTP(out, r)
		return out
	}
	first := post(`{"elementNo":1,"seqNo":0,"i":[1],"q":[0],"sampleRate":48000}`)
	if first.Code != http.StatusCreated { t.Fatalf("first upload status=%d body=%s", first.Code, first.Body.String()) }
	second := post(`{"elementNo":1,"seqNo":0,"i":[0],"q":[1],"sampleRate":48000}`)
	if second.Code != http.StatusConflict { t.Fatalf("conflicting duplicate status=%d body=%s", second.Code, second.Body.String()) }
}
