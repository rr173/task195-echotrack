package httpapi

import (
	"net/http"
	"strings"
	"time"
)

// middleware 提供请求增强：记录耗时、允许 CORS。
// 挂载在 Server.Handler 外层（见 Handler()）。

// loggingMiddleware 记录请求耗时。
func loggingMiddleware(next http.Handler, logger interface{ Printf(string, ...any) }) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Microsecond))
	})
}

// corsMiddleware 允许跨域 JSON 请求（前端页面同源场景可关闭）。
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if strings.EqualFold(r.Method, http.MethodOptions) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusWriter 捕获写入状态码。
type statusWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader 记录状态码。
func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
