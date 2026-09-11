package blocklist

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/shivang-16/orbit.api/internal/model"
)

//go:embed domains.txt
var domainsFile string

const ContactEmail = "shivang@tryorbit.cloud"

const (
	ErrorCode    = "user_blocked"
	ErrorMessage = "You are blocked on this site. Please contact " + ContactEmail + " for more."
)

var domains = parseDomains(domainsFile)

func parseDomains(raw string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for line := range strings.SplitSeq(raw, "\n") {
		domain := normalizeDomain(line)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}

func normalizeDomain(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") {
		return ""
	}
	value = strings.TrimPrefix(value, "@")
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "*.")
	return value
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
	if domain == "" {
		return false
	}
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
