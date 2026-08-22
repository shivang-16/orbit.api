// Package apikey authenticates inference requests made with an Orbit API
// key (sk-orbit-...), as opposed to a Clerk session JWT. It resolves the
// key to an organization and attaches that to the request context.
package apikey

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shivang-16/orbit.api/internal/logger"
	"github.com/shivang-16/orbit.api/internal/model"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
)

type contextKey string

const (
	organizationIDKey contextKey = "api_key_organization_id"
	apiKeyIDKey       contextKey = "api_key_id"
)

type Middleware struct {
	keys  *apikeyRepository.Repository
	users *userRepository.Repository
}

func New(keys *apikeyRepository.Repository, users *userRepository.Repository) *Middleware {
	return &Middleware{keys: keys, users: users}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := credential(r)
		if secret == "" {
			writeError(w, http.StatusUnauthorized, "missing api key")
			return
		}

		ctx := r.Context()
		item, err := m.keys.GetActiveByHash(ctx, apikeyService.HashSecret(secret))
		if err != nil {
			logger.Error(ctx, "apikey: lookup failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to validate api key")
			return
		}
		// GetActiveByHash already requires status=active, revoked_at IS NULL,
		// and a non-expired key. Re-check status here so an inactive key can
		// never authenticate even if the query is later loosened.
		if item == nil || item.Status != model.APIKeyStatusActive {
			logger.Warn(ctx, "apikey: invalid or inactive api key")
			writeError(w, http.StatusUnauthorized, "invalid api key")
			return
		}

		// Detached from the request context so cancellation on response
		// flush doesn't race with (or skip) recording usage.
		go func(id string) {
			_ = m.keys.TouchLastUsed(context.WithoutCancel(ctx), id)
		}(item.ID)

		ctx = context.WithValue(ctx, organizationIDKey, item.OrganizationID)
		ctx = context.WithValue(ctx, apiKeyIDKey, item.ID)
		ctx = logger.SetOrg(ctx, item.OrganizationID)
		if m.users != nil && item.CreatedBy != "" {
			if user, lookupErr := m.users.GetByID(ctx, item.CreatedBy); lookupErr != nil {
				logger.Warn(ctx, "apikey: owner email lookup failed", "user_id", item.CreatedBy, "error", lookupErr)
			} else if user != nil {
				ctx = logger.SetUser(ctx, user.ID, user.Email)
			}
		}
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

// WithOrganization attaches an organization to the request context the
// same way Authenticate does, without an API key. Used by the dashboard
// playground so session-authenticated chat is billed to the active org.
func WithOrganization(ctx context.Context, organizationID string) context.Context {
	ctx = context.WithValue(ctx, organizationIDKey, organizationID)
	return logger.SetOrg(ctx, organizationID)
}

// credential extracts the Orbit API key from a request, accepting both
// "Authorization: Bearer sk-orbit-..." (the OpenAI SDK and Orbit's own
// convention) and "x-api-key: sk-orbit-..." (the Anthropic SDK's default),
// so either SDK authenticates without extra configuration.
func credential(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	return strings.TrimSpace(r.Header.Get("X-Api-Key"))
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
