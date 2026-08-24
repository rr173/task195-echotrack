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

// TestUploadWindowConflictOnMismatchedReupload 验证：同一批次同一阵元同一序号的窗口
// 再次上传且内容不同时，接口必须返回 409 Conflict；而相同内容重复上传仍幂等（201）。
func TestUploadWindowConflictOnMismatchedReupload(t *testing.T) {
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

	mustPost := func(t *testing.T, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}

	mustPost(t, "/api/arrays", `{"id":"arr-conf","name":"conf","sampleRateHz":48000,"elementCount":2}`)
	mustPost(t, "/api/batches", `{"id":"b-conf","arrayId":"arr-conf","referenceElement":1}`)

	window := `{"elementNo":1,"seqNo":0,"sampleRate":48000,"i":[1,2,3],"q":[4,5,6]}`

	// 首次上传：201。
	first := mustPost(t, "/api/batches/b-conf/windows", window)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: got %d, want 201, body=%s", first.Code, first.Body.String())
	}

	// 相同内容重复上传：幂等，仍 201。
	again := mustPost(t, "/api/batches/b-conf/windows", window)
	if again.Code != http.StatusCreated {
		t.Fatalf("idempotent reupload: got %d, want 201, body=%s", again.Code, again.Body.String())
	}

	// 内容不同的重复上传：必须 409 Conflict，不得降级为 500。
	mismatch := `{"elementNo":1,"seqNo":0,"sampleRate":48000,"i":[1,2,3],"q":[4,5,9]}`
	conflict := mustPost(t, "/api/batches/b-conf/windows", mismatch)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("mismatched reupload: got %d, want 409 Conflict, body=%s", conflict.Code, conflict.Body.String())
	}
}
