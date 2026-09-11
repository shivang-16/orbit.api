package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2/jwt"

	"github.com/shivang-16/orbit.api/internal/blocklist"
	"github.com/shivang-16/orbit.api/internal/logger"
	"github.com/shivang-16/orbit.api/internal/model"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
)

type contextKey string

const userIDKey contextKey = "user_id"

type Middleware struct {
	users *userRepository.Repository
}

func New(users *userRepository.Repository) *Middleware {
	return &Middleware{users: users}
}

func (m *Middleware) Clerk(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			logger.Warn(ctx, "auth: missing bearer token")
			writeUnauthorized(w)
			return
		}

		claims, err := jwt.Verify(ctx, &jwt.VerifyParams{Token: token})
		if err != nil {
			logger.Warn(ctx, "auth: jwt verify failed", "error", err)
			writeUnauthorized(w)
			return
		}

		ctx = context.WithValue(ctx, userIDKey, claims.Subject)
		var user *model.User
		if m.users != nil {
			lookup, lookupErr := m.users.GetByID(ctx, claims.Subject)
			if lookupErr != nil {
				logger.Error(ctx, "auth: user lookup failed", "user_id", claims.Subject, "error", lookupErr)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to authorize"})
				return
			}
			user = lookup
		}

		if rejectBlockedUser(w, ctx, m.users, user) {
			return
		}

		// New accounts may only sync. Every other Clerk route requires an
		// existing, unblocked user so a blocked-domain signup cannot create
		// orgs, keys, or playground inference before /users/sync runs.
		if user == nil && !isUserSync(r) {
			logger.Warn(ctx, "auth: user not synced", "user_id", claims.Subject)
			writeUnauthorized(w)
			return
		}

		email := ""
		if user != nil {
			email = user.Email
		}
		ctx = logger.SetUser(ctx, claims.Subject, email)
		if orgID := strings.TrimSpace(r.Header.Get("X-Organization-Id")); orgID != "" {
			ctx = logger.SetOrg(ctx, orgID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func UserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func isUserSync(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/users/sync")
}

func writeUnauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
}

func writeJSON(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func rejectBlockedUser(w http.ResponseWriter, ctx context.Context, users *userRepository.Repository, user *model.User) bool {
	if !blocklist.UserIsBlocked(user) {
		return false
	}
	if !user.Blocked && users != nil {
		if err := users.SetBlocked(ctx, user.ID, true); err != nil {
			logger.Warn(ctx, "auth: mark blocked failed", "user_id", user.ID, "error", err)
		}
	}
	logger.Warn(ctx, "auth: blocked user", "user_id", user.ID, "email", user.Email)
	blocklist.WriteForbidden(w)
	return true
}
