package logger

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// HTTP logs each request after it finishes so auth has already attached
// email / user / org. Inference routes get tag=inference from BindRequest.
// The message is the route itself so Better Stack shows more than "http request".
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := BindRequest(r.Context(), r.Method, r.URL.Path)
		r = r.WithContext(ctx)
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)

		path := r.URL.Path
		if path == "/health" || path == "/ready" {
			return
		}

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		Info(r.Context(), fmt.Sprintf("%s %s %d", r.Method, path, status),
			"status", status,
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
