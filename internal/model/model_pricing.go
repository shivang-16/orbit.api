package model

import "time"

// ModelPricing stores on-demand rates per 1 million tokens, in USD micros
// (1_000_000 micros = $1.00). Vendor rates are Bedrock list prices; Orbit
// rates are what we charge the organization.
type ModelPricing struct {
	ID                           string    `json:"id" db:"id"`
	ModelCatalogueID             string    `json:"model_catalogue_id" db:"model_catalogue_id"`
	VendorInputPerMillionMicros  int64     `json:"vendor_input_per_million_micros" db:"vendor_input_per_million_micros"`
	VendorOutputPerMillionMicros int64     `json:"vendor_output_per_million_micros" db:"vendor_output_per_million_micros"`
	OrbitInputPerMillionMicros   int64     `json:"orbit_input_per_million_micros" db:"orbit_input_per_million_micros"`
	OrbitOutputPerMillionMicros  int64     `json:"orbit_output_per_million_micros" db:"orbit_output_per_million_micros"`
	Currency                     string    `json:"currency" db:"currency"`
	CreatedAt                    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at" db:"updated_at"`
}
