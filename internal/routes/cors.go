package routes

import (
	"net/http"
	"strings"

	"github.com/go-chi/cors"

	"github.com/shivang-16/orbit.api/internal/config"
)

// corsByPath must sit on the root router (r.Use), not on a method-specific
// group. Chi only runs group middleware when a route matches; OPTIONS is
// never registered on POST-only inference routes, so group-level CORS
// never sees the browser preflight and Chi returns 405 in tens of
// microseconds. Root middleware runs first and the cors handler answers
// OPTIONS itself.
func corsByPath(cfg config.Config) func(http.Handler) http.Handler {
	dashboard := cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Organization-Id"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	apiKey := cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	})

	return func(next http.Handler) http.Handler {
		dashboardNext := dashboard(next)
		apiKeyNext := apiKey(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAPIKeyRoute(r.URL.Path) {
				apiKeyNext.ServeHTTP(w, r)
				return
			}
			dashboardNext.ServeHTTP(w, r)
		})
	}
}

func isAPIKeyRoute(path string) bool {
	switch path {
	case "/api/v1/chat/completions", "/api/v1/messages", "/api/v1/models":
		return true
	}

	const prefix = "/api/v1/models/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, "/chat") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/chat")
	return id != "" && !strings.Contains(id, "/")
}
