package httpapi

import (
	"bytes"
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

// setupUploadServer 注册阵列并创建上传中批次，返回就绪服务器与 batchID。
func setupUploadServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/echotrack.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	arr := service.NewArrayService(s)
	batch := service.NewBatchService(s, arr)
	proc := service.NewProcessService(s, arr)
	rev := service.NewReviewService(s)
	h := NewHandler(arr, batch, proc, rev)
	srv := New(h, log.New(io.Discard, "", 0))

	bg := context.Background()
	if _, err := arr.Register(bg, "arr-x", "x", 48000, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := batch.Create(bg, "batch-x", "arr-x", 1); err != nil {
		t.Fatal(err)
	}
	return srv, "batch-x"
}

// TestUploadWindowCancelledReturnsIdentifiableResult 客户端在上传前取消请求：
// 返回 499 + cancelled:true 的可识别取消结果，且后续 GET windows 看不到该窗口。
func TestUploadWindowCancelledReturnsIdentifiableResult(t *testing.T) {
	srv, batchID := setupUploadServer(t)
	win := `{"elementNo":1,"seqNo":0,"i":[0.1,0.2],"q":[1,2],"sampleRate":48000}`

	// 用已取消的 ctx 发起上传。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/batches/"+batchID+"/windows", bytes.NewReader([]byte(win))).WithContext(ctx)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != 499 {
		t.Fatalf("expected 499 for cancelled upload, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cancelled":true`) {
		t.Fatalf("response should mark cancelled=true: %s", w.Body.String())
	}

	// 后续查询看不到该窗口。
	listReq := httptest.NewRequest(http.MethodGet, "/api/batches/"+batchID+"/windows", nil).WithContext(context.Background())
	lw := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lw, listReq)
	if lw.Code != http.StatusOK {
		t.Fatalf("list windows: %d %s", lw.Code, lw.Body.String())
	}
	if strings.Contains(lw.Body.String(), `"seqNo":0`) {
		t.Fatalf("cancelled window must not appear in list: %s", lw.Body.String())
	}
}

// TestUploadWindowNormalSucceeds 非取消上传成功并返回 201，回归保护。
func TestUploadWindowNormalSucceeds(t *testing.T) {
	srv, batchID := setupUploadServer(t)
	win := `{"elementNo":1,"seqNo":0,"i":[0.1,0.2],"q":[1,2],"sampleRate":48000}`
	req := httptest.NewRequest(http.MethodPost, "/api/batches/"+batchID+"/windows", bytes.NewReader([]byte(win))).WithContext(context.Background())
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}
