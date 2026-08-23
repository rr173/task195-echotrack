// Package httpapi 提供声学阵列回波相位追踪台的 JSON HTTP API。
// 路由统一以 /api 开头，全部使用标准库 net/http ServeMux（Go 1.22+ 方法路由）。
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// Server HTTP 服务器。
type Server struct {
	mux     *http.ServeMux
	handler *Handler
	logger  *log.Logger
}

// New 构造 HTTP 服务器并注册全部路由。
func New(h *Handler, logger *log.Logger) *Server {
	s := &Server{mux: http.NewServeMux(), handler: h, logger: logger}
	s.routes()
	return s
}

// Handler 返回底层处理器（含日志与 CORS 中间件）。
func (s *Server) Handler() http.Handler {
	return corsMiddleware(loggingMiddleware(s.mux, s.logger))
}

// routes 注册全部 API 路由。
func (s *Server) routes() {
	// 阵列
	s.mux.HandleFunc("POST /api/arrays", s.withLog(s.handler.CreateArray))
	s.mux.HandleFunc("GET /api/arrays", s.withLog(s.handler.ListArrays))
	s.mux.HandleFunc("GET /api/arrays/{id}", s.withLog(s.handler.GetArray))
	s.mux.HandleFunc("GET /api/arrays/{id}/stats", s.withLog(s.handler.ArrayStats))
	s.mux.HandleFunc("PUT /api/arrays/{id}/elements/{elementNo}/delay", s.withLog(s.handler.UpdateElementDelay))
	// 批次
	s.mux.HandleFunc("POST /api/batches", s.withLog(s.handler.CreateBatch))
	s.mux.HandleFunc("GET /api/batches", s.withLog(s.handler.ListBatches))
	s.mux.HandleFunc("GET /api/batches/{id}", s.withLog(s.handler.GetBatch))
	s.mux.HandleFunc("POST /api/batches/{id}/windows", s.withLog(s.handler.UploadWindow))
	s.mux.HandleFunc("GET /api/batches/{id}/windows", s.withLog(s.handler.ListWindows))
	s.mux.HandleFunc("GET /api/batches/{id}/windows/{windowId}", s.withLog(s.handler.GetWindow))
	s.mux.HandleFunc("POST /api/batches/{id}/windows/{elementNo}/{seqNo}/ignore", s.withLog(s.handler.IgnoreWindow))
	s.mux.HandleFunc("POST /api/batches/{id}/process", s.withLog(s.handler.ProcessBatch))
	s.mux.HandleFunc("POST /api/batches/{id}/reference/{elementNo}", s.withLog(s.handler.ChangeReference))
	// 轨迹
	s.mux.HandleFunc("GET /api/batches/{id}/tracks", s.withLog(s.handler.ListTracks))
	s.mux.HandleFunc("GET /api/tracks/{trackId}", s.withLog(s.handler.GetTrack))
	s.mux.HandleFunc("GET /api/tracks/{trackId}/segments", s.withLog(s.handler.ListSegments))
	s.mux.HandleFunc("POST /api/tracks/{trackId}/segments", s.withLog(s.handler.AddSegment))
	s.mux.HandleFunc("POST /api/tracks/{trackId}/confirm", s.withLog(s.handler.ConfirmTrack))
	// 标注
	s.mux.HandleFunc("POST /api/batches/{id}/annotations", s.withLog(s.handler.AddAnnotation))
	s.mux.HandleFunc("GET /api/batches/{id}/annotations", s.withLog(s.handler.ListAnnotations))
	// 解释包
	s.mux.HandleFunc("POST /api/batches/{id}/interpretations", s.withLog(s.handler.CreateInterpretation))
	s.mux.HandleFunc("GET /api/batches/{id}/interpretations", s.withLog(s.handler.ListInterpretations))
	s.mux.HandleFunc("POST /api/interpretations/{interpretationId}/publish", s.withLog(s.handler.PublishInterpretation))
	s.mux.HandleFunc("POST /api/interpretations/supersede", s.withLog(s.handler.SupersedeInterpretation))
	// 系统
	s.mux.HandleFunc("GET /api/stats", s.withLog(s.handler.SystemStats))
	s.mux.HandleFunc("GET /api/health", s.withLog(s.handler.Health))
}

// withLog 包装处理器：记录方法与路径，统一 panic 恢复。
func (s *Server) withLog(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		s.logger.Printf("%s %s", r.Method, r.URL.Path)
		next(w, r)
	}
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// 编码失败时仅记录。
		return
	}
}

// apiError 错误响应体。
type apiError struct {
	Error string `json:"error"`
}

// writeError 输出错误响应。
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// parseBody 解析 JSON 请求体。
func parseBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return false
	}
	return true
}

// pathValue 读取路径参数。
func pathValue(r *http.Request, key string) string {
	return strings.TrimSpace(r.PathValue(key))
}
