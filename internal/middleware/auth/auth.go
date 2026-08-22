package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2/jwt"

	"github.com/shivang-16/orbit.api/internal/logger"
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
		email := ""
		if m.users != nil {
			if user, lookupErr := m.users.GetByID(ctx, claims.Subject); lookupErr != nil {
				logger.Warn(ctx, "auth: user email lookup failed", "user_id", claims.Subject, "error", lookupErr)
			} else if user != nil {
				email = user.Email
			}
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

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
