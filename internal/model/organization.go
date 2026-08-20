package model

import "time"

type Organization struct {
	ID                     string    `json:"id" db:"id"`
	Name                   string    `json:"name" db:"name"`
	Slug                   string    `json:"slug" db:"slug"`
	Description            string    `json:"description" db:"description"`
	CreatedBy              string    `json:"created_by" db:"created_by"`
	PlanSlug               string    `json:"plan_slug,omitempty" db:"plan_slug"`
	CreditsGrantedMicros   int64     `json:"credits_granted_micros" db:"credits_granted_micros"`
	CreditsUsedMicros      int64     `json:"credits_used_micros" db:"credits_used_micros"`
	CreditsRemainingMicros int64     `json:"credits_remaining_micros" db:"credits_remaining_micros"`
	CreatedAt              time.Time `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time `json:"updated_at" db:"updated_at"`
}
