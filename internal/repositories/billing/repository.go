package billing

import (
	"context"
	"database/sql"
	"fmt"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

type RecordParams struct {
	IdempotencyKey     string
	OrganizationID     string
	APIKeyID           string
	ModelCatalogueID   string
	Prompt             string
	InputTokens        int
	OutputTokens       int
	LatencyMS          int
	Status             string
	Error              string
	AmountMicros       int64
	VendorAmountMicros int64
	OrbitAmountMicros  int64
}

// Record writes the inference row, a usage ledger entry, and bumps the org
// credit cache in one transaction so a crash cannot charge without a log.
func (r *Repository) Record(ctx context.Context, params RecordParams) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin billing tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var apiKeyID, modelID sql.NullString
	if params.APIKeyID != "" {
		apiKeyID = sql.NullString{String: params.APIKeyID, Valid: true}
	}
	if params.ModelCatalogueID != "" {
		modelID = sql.NullString{String: params.ModelCatalogueID, Valid: true}
	}

	idempotencyKey := params.IdempotencyKey
	if idempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}

	var requestID string
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO inference_requests (
			organization_id, api_key_id, model_catalogue_id, prompt,
			input_tokens, output_tokens, latency_ms, status, error, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING id`,
		params.OrganizationID,
		apiKeyID,
		modelID,
		params.Prompt,
		params.InputTokens,
		params.OutputTokens,
		params.LatencyMS,
		params.Status,
		params.Error,
		idempotencyKey,
	).Scan(&requestID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(
			ctx,
			`SELECT id FROM inference_requests WHERE idempotency_key = $1`,
			idempotencyKey,
		).Scan(&requestID)
	}
	if err != nil {
		return fmt.Errorf("insert inference request: %w", err)
	}

	if params.AmountMicros > 0 && params.Status == "success" {
		result, err := tx.ExecContext(
			ctx,
			`INSERT INTO credit_ledger (
				organization_id, inference_request_id, entry_type,
				amount_micros, vendor_amount_micros, orbit_amount_micros,
				idempotency_key, note
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (idempotency_key) DO NOTHING`,
			params.OrganizationID,
			requestID,
			"usage",
			params.AmountMicros,
			params.VendorAmountMicros,
			params.OrbitAmountMicros,
			"usage:"+idempotencyKey,
			"",
		)
		if err != nil {
			return fmt.Errorf("insert credit ledger: %w", err)
		}

		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("credit ledger rows: %w", err)
		}
		if inserted == 0 {
			return tx.Commit()
		}

		_, err = tx.ExecContext(
			ctx,
			`UPDATE organizations
			 SET credits_used_micros = credits_used_micros + $1,
			     credits_remaining_micros = GREATEST(credits_remaining_micros - $1, 0)
			 WHERE id = $2`,
			params.AmountMicros,
			params.OrganizationID,
		)
		if err != nil {
			return fmt.Errorf("update organization credits: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit billing tx: %w", err)
	}
	return nil
}
