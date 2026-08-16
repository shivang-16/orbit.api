package model

import (
	"time"

	"github.com/lib/pq"
)

type Plan struct {
	ID            string         `json:"id" db:"id"`
	Slug          string         `json:"slug" db:"slug"`
	Name          string         `json:"name" db:"name"`
	DodoProductID string         `json:"dodo_product_id" db:"dodo_product_id"`
	PriceMicros   int64          `json:"price_micros" db:"price_micros"`
	CreditsMicros int64          `json:"credits_micros" db:"credits_micros"`
	Tagline       string         `json:"tagline" db:"tagline"`
	Features      pq.StringArray `json:"features" db:"features"`
	IncludesFrom  string         `json:"includes_from" db:"includes_from"`
	Highlighted   bool           `json:"highlighted" db:"highlighted"`
	SortOrder     int            `json:"sort_order" db:"sort_order"`
	IsActive      bool           `json:"is_active" db:"is_active"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at" db:"updated_at"`
}
