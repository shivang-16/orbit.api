package logger

import (
	"context"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

type contextKey struct{}

// Fields is the mutable per-request bag, same idea as Choppr's
// AsyncLocalStorage request context. Auth middleware fills email/user/org
// so every later log line carries them to Better Stack.
type Fields struct {
	RequestID string
	Method    string
	Path      string
	UserID    string
	Email     string
	OrgID     string
	Tag       string
}

func Ensure(ctx context.Context) (*Fields, context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if f, ok := ctx.Value(contextKey{}).(*Fields); ok && f != nil {
		return f, ctx
	}
	f := &Fields{}
	return f, context.WithValue(ctx, contextKey{}, f)
}

func From(ctx context.Context) *Fields {
	if ctx == nil {
		return &Fields{}
	}
	if f, ok := ctx.Value(contextKey{}).(*Fields); ok && f != nil {
		return f
	}
	return &Fields{}
}

func SetUser(ctx context.Context, userID, email string) context.Context {
	f, ctx := Ensure(ctx)
	f.UserID = strings.TrimSpace(userID)
	f.Email = strings.TrimSpace(email)
	return ctx
}

func SetOrg(ctx context.Context, orgID string) context.Context {
	f, ctx := Ensure(ctx)
	f.OrgID = strings.TrimSpace(orgID)
	return ctx
}

func SetTag(ctx context.Context, tag string) context.Context {
	f, ctx := Ensure(ctx)
	f.Tag = strings.TrimSpace(tag)
	return ctx
}

func BindRequest(ctx context.Context, method, path string) context.Context {
	f, ctx := Ensure(ctx)
	f.Method = method
	f.Path = path
	if id := middleware.GetReqID(ctx); id != "" {
		f.RequestID = id
	}
	if isInferencePath(path) {
		f.Tag = TagInference
	} else {
		f.Tag = TagAPI
	}
	return ctx
}

const (
	TagInference = "inference"
	TagAPI       = "api"
	TagMail      = "mail"
	TagBilling   = "billing"
)

func isInferencePath(path string) bool {
	switch path {
	case "/api/v1/chat/completions", "/api/v1/messages":
		return true
	}
	if strings.HasPrefix(path, "/api/v1/playground/models/") && strings.HasSuffix(path, "/chat") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/models/") && strings.HasSuffix(path, "/chat") {
		return true
	}
	return false
}

func attrs(ctx context.Context) []any {
	f := From(ctx)
	args := make([]any, 0, 14)
	if f.Tag != "" {
		args = append(args, "tag", f.Tag)
	}
	if f.Email != "" {
		args = append(args, "email", f.Email, "userEmail", f.Email)
	}
	if f.UserID != "" {
		args = append(args, "user_id", f.UserID)
	}
	if f.OrgID != "" {
		args = append(args, "org_id", f.OrgID)
	}
	if f.RequestID != "" {
		args = append(args, "request_id", f.RequestID)
	}
	if f.Method != "" {
		args = append(args, "method", f.Method)
	}
	if f.Path != "" {
		args = append(args, "path", f.Path)
	}
	return args
}
