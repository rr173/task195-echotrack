package httpapi

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"task195-echotrack/internal/service"
	"task195-echotrack/internal/store"
)

func TestServerRoutesHealthAndArrayWrite(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	arr := service.NewArrayService(s)
	batch := service.NewBatchService(s, arr)
	proc := service.NewProcessService(s, arr)
	rev := service.NewReviewService(s)
	h := NewHandler(arr, batch, proc, rev)
	srv := New(h, log.New(io.Discard, "", 0))

	health := httptest.NewRecorder()
	srv.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"status":"ok"`) {
		t.Fatalf("health response: code=%d body=%s", health.Code, health.Body.String())
	}

	body := strings.NewReader(`{"id":"arr-http-test","name":"http test","sampleRateHz":48000,"elementCount":2}`)
	created := httptest.NewRecorder()
	srv.Handler().ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/arrays", body))
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":"arr-http-test"`) {
		t.Fatalf("array response: code=%d body=%s", created.Code, created.Body.String())
	}
}
