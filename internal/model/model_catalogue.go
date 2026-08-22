package model

import (
	"time"

	"github.com/lib/pq"
)

type ModelCatalogue struct {
	ID                string         `json:"id" db:"id"`
	Name              string         `json:"name" db:"name"`
	Slug              string         `json:"slug" db:"slug"`
	Vendor            string         `json:"vendor" db:"vendor"`
	Provider          string         `json:"provider" db:"provider"`
	ModelID           string         `json:"model_id" db:"model_id"`
	InputContextLimit int            `json:"input_context_limit" db:"input_context_limit"`
	SortOrder         int            `json:"sort_order" db:"sort_order"`
	Tags              pq.StringArray `json:"tags" db:"tags"`
	Modalities        pq.StringArray `json:"modalities" db:"modalities"`
	IsActive          bool           `json:"is_active" db:"is_active"`
	ModelReleasedDate *time.Time     `json:"model_released_date" db:"model_released_date"`
	CreatedAt         time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at" db:"updated_at"`
}
