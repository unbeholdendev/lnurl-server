package server

import (
	"log"
	"net/http"
	"time"
)

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK // default if WriteHeader was never called
	}
	return w.status
}

// requestLogging returns middleware that logs each request's method, path, status, duration, and client IP.
func requestLogging(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusResponseWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s %d %s %s", r.Method, r.URL.Path, sw.statusCode(), time.Since(start).Round(time.Millisecond), clientIP(r))
	})
}
