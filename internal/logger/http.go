package logger

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// HTTP logs each request after it finishes so auth has already attached
// email / user / org. Inference routes get tag=inference from BindRequest.
func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := BindRequest(r.Context(), r.Method, r.URL.Path)
		r = r.WithContext(ctx)
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		Info(r.Context(), "http request",
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
