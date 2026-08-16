package model

import "time"

type APIKey struct {
	ID             string     `json:"id" db:"id"`
	OrganizationID string     `json:"organization_id" db:"organization_id"`
	CreatedBy      string     `json:"created_by" db:"created_by"`
	Name           string     `json:"name" db:"name"`
	KeyPreview     string     `json:"key_preview" db:"key_preview"`
	ExpiresAt      *time.Time `json:"expires_at" db:"expires_at"`
	LastUsedAt     *time.Time `json:"last_used_at" db:"last_used_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}
