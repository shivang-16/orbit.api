// Package apikey authenticates inference requests made with an Orbit API
// key (sk-orbit-...), as opposed to a Clerk session JWT. It resolves the
// key to an organization and attaches that to the request context.
package apikey

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
)

type contextKey string

const (
	organizationIDKey contextKey = "api_key_organization_id"
	apiKeyIDKey       contextKey = "api_key_id"
)

type Middleware struct {
	keys *apikeyRepository.Repository
}

func New(keys *apikeyRepository.Repository) *Middleware {
	return &Middleware{keys: keys}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := bearerToken(r.Header.Get("Authorization"))
		if secret == "" {
			writeError(w, http.StatusUnauthorized, "missing api key")
			return
		}

		item, err := m.keys.GetActiveByHash(r.Context(), apikeyService.HashSecret(secret))
		if err != nil {
			log.Printf("apikey: lookup failed: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to validate api key")
			return
		}
		if item == nil {
			log.Printf("apikey: invalid or expired key path=%s", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "invalid or expired api key")
			return
		}

		// Detached from the request context so cancellation on response
		// flush doesn't race with (or skip) recording usage.
		go func(id string) {
			_ = m.keys.TouchLastUsed(context.WithoutCancel(r.Context()), id)
		}(item.ID)

		ctx := context.WithValue(r.Context(), organizationIDKey, item.OrganizationID)
		ctx = context.WithValue(ctx, apiKeyIDKey, item.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func OrganizationID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(organizationIDKey).(string)
	return id, ok && id != ""
}

func APIKeyID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(apiKeyIDKey).(string)
	return id, ok && id != ""
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
