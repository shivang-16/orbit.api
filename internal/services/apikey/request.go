package apikey

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ExpirationNever = "never"
	Expiration30d   = "30d"
	Expiration90d   = "90d"
	Expiration1y    = "1y"
)

type CreateRequest struct {
	Name           string `json:"name"`
	Expiration     string `json:"expiration"`
	OrganizationID string `json:"organization_id"`
}

func (r *CreateRequest) isValid() bool {
	r.Name = strings.TrimSpace(r.Name)
	r.OrganizationID = strings.TrimSpace(r.OrganizationID)
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 80 {
		return false
	}

	r.Expiration = strings.TrimSpace(strings.ToLower(r.Expiration))
	if r.Expiration == "" {
		r.Expiration = ExpirationNever
	}

	switch r.Expiration {
	case ExpirationNever, Expiration30d, Expiration90d, Expiration1y:
		return true
	default:
		return false
	}
}

func (r CreateRequest) expiresAt(now time.Time) *time.Time {
	var until time.Duration
	switch r.Expiration {
	case Expiration30d:
		until = 30 * 24 * time.Hour
	case Expiration90d:
		until = 90 * 24 * time.Hour
	case Expiration1y:
		until = 365 * 24 * time.Hour
	default:
		return nil
	}
	value := now.Add(until)
	return &value
}
