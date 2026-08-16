package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
				amount_micros, vendor_amount_micros, idempotency_key, note
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (idempotency_key) DO NOTHING`,
			params.OrganizationID,
			requestID,
			"usage",
			params.AmountMicros,
			params.VendorAmountMicros,
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

type GrantParams struct {
	OrganizationID string
	AmountMicros   int64
	IdempotencyKey string
	Note           string
}

// GrantCredits adds a positive ledger entry (plan purchase, top-up, manual
// grant) and bumps the org's granted/remaining balance. Idempotent on
// IdempotencyKey so a retried webhook delivery never double-grants.
func (r *Repository) GrantCredits(ctx context.Context, params GrantParams) error {
	if params.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}
	if params.AmountMicros <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin grant tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO credit_ledger (
			organization_id, entry_type, amount_micros, idempotency_key, note
		) VALUES ($1, 'grant', $2, $3, $4)
		 ON CONFLICT (idempotency_key) DO NOTHING`,
		params.OrganizationID,
		params.AmountMicros,
		params.IdempotencyKey,
		params.Note,
	)
	if err != nil {
		return fmt.Errorf("insert credit ledger grant: %w", err)
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
		 SET credits_granted_micros = credits_granted_micros + $1,
		     credits_remaining_micros = credits_remaining_micros + $1
		 WHERE id = $2`,
		params.AmountMicros,
		params.OrganizationID,
	)
	if err != nil {
		return fmt.Errorf("update organization credits: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit grant tx: %w", err)
	}
	return nil
}

type HistoryRow struct {
	ID                 string
	OrganizationID     string
	InferenceRequestID sql.NullString
	EntryType          string
	AmountMicros       int64
	VendorAmountMicros int64
	IdempotencyKey     string
	Note               string
	CreatedAt          time.Time
	ModelName          string
	InputTokens        int
	OutputTokens       int
}

func (r *Repository) ListHistory(ctx context.Context, organizationID string) ([]HistoryRow, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT cl.id, cl.organization_id, cl.inference_request_id, cl.entry_type,
		        cl.amount_micros, cl.vendor_amount_micros, cl.idempotency_key, cl.note,
		        cl.created_at,
		        COALESCE(mc.name, ''),
		        COALESCE(ir.input_tokens, 0),
		        COALESCE(ir.output_tokens, 0)
		 FROM credit_ledger cl
		 LEFT JOIN inference_requests ir ON ir.id = cl.inference_request_id
		 LEFT JOIN model_catalogue mc ON mc.id = ir.model_catalogue_id
		 WHERE cl.organization_id = $1
		 ORDER BY cl.created_at DESC
		 LIMIT 200`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]HistoryRow, 0)
	for rows.Next() {
		var row HistoryRow
		if err := rows.Scan(
			&row.ID,
			&row.OrganizationID,
			&row.InferenceRequestID,
			&row.EntryType,
			&row.AmountMicros,
			&row.VendorAmountMicros,
			&row.IdempotencyKey,
			&row.Note,
			&row.CreatedAt,
			&row.ModelName,
			&row.InputTokens,
			&row.OutputTokens,
		); err != nil {
			return nil, err
		}
		entries = append(entries, row)
	}
	return entries, rows.Err()
}
