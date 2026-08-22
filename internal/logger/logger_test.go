package logger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsInferencePath(t *testing.T) {
	cases := map[string]bool{
		"/api/v1/chat/completions":                       true,
		"/api/v1/messages":                               true,
		"/api/v1/models/claude-sonnet-5/chat":            true,
		"/api/v1/playground/models/claude-sonnet-5/chat": true,
		"/api/v1/models":                                 false,
		"/api/v1/users/sync":                             false,
		"/health":                                        false,
	}
	for path, want := range cases {
		if got := isInferencePath(path); got != want {
			t.Fatalf("isInferencePath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestSetUserPersistsOnNewContext(t *testing.T) {
	ctx := SetUser(context.Background(), "user_1", "dev@tryorbit.cloud")
	f := From(ctx)
	if f.Email != "dev@tryorbit.cloud" || f.UserID != "user_1" {
		t.Fatalf("fields = %+v", f)
	}
	args := attrs(ctx)
	foundEmail, foundUserEmail := false, false
	for i := 0; i+1 < len(args); i += 2 {
		key, _ := args[i].(string)
		val, _ := args[i+1].(string)
		if key == "email" && val == "dev@tryorbit.cloud" {
			foundEmail = true
		}
		if key == "userEmail" && val == "dev@tryorbit.cloud" {
			foundUserEmail = true
		}
	}
	if !foundEmail || !foundUserEmail {
		t.Fatalf("email attrs missing: %v", args)
	}
}

func TestHTTPTagsInference(t *testing.T) {
	var tagged string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tagged = From(r.Context()).Tag
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	HTTP(inner).ServeHTTP(rr, req)
	if tagged != TagInference {
		t.Fatalf("tag = %q, want %q", tagged, TagInference)
	}
}
