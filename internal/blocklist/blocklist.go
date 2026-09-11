package blocklist

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/shivang-16/orbit.api/internal/model"
)

const ContactEmail = "shivang@tryorbit.cloud"

const (
	ErrorCode    = "user_blocked"
	ErrorMessage = "You are blocked on this site. Please contact " + ContactEmail + " for more."
)

var mu sync.RWMutex
var domains []string

func NormalizeDomain(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") {
		return ""
	}
	value = strings.TrimPrefix(value, "@")
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "*.")
	value = strings.Trim(value, ".")
	return value
}

func SetDomains(values []string) {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		domain := NormalizeDomain(value)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	mu.Lock()
	domains = out
	mu.Unlock()
}

func EmailBlocked(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	_, domain, ok := strings.Cut(email, "@")
	if !ok || domain == "" {
		return false
	}
	return DomainBlocked(domain)
}

func UserIsBlocked(user *model.User) bool {
	return user != nil && (user.Blocked || EmailBlocked(user.Email))
}

func DomainBlocked(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.Trim(domain, ".")
	if domain == "" {
		return false
	}
	mu.RLock()
	defer mu.RUnlock()
	for _, blocked := range domains {
		if domain == blocked || strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
}

func WriteForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "blocked",
		"code":    ErrorCode,
		"message": ErrorMessage,
	})
}
