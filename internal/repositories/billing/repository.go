package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type dbTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

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
	HoldID             string
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

	ledgerInserted := int64(0)
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

		ledgerInserted, err = result.RowsAffected()
		if err != nil {
			return fmt.Errorf("credit ledger rows: %w", err)
		}
	}

	actual := params.AmountMicros
	if params.Status != "success" {
		actual = 0
	}

	if params.HoldID != "" {
		// actual is the same AmountMicros written to credit_ledger above,
		// so remaining/used stay aligned with the ledger after this tx.
		settled, err := settleHoldTx(ctx, tx, params.HoldID, actual)
		if err != nil {
			return err
		}
		// Sweeper may have already refunded an expired hold (crash or
		// SQS retry past TTL). The freeze is back on remaining; if this
		// attempt is the first ledger insert, take actual usage now so
		// remaining/used match the ledger. A retry sees ledgerInserted=0
		// and skips.
		if !settled.Applied && settled.Found && ledgerInserted > 0 && actual > 0 {
			if err := debitUsageTx(ctx, tx, settled.OrganizationID, actual); err != nil {
				return err
			}
		}
	} else if ledgerInserted > 0 {
		// Legacy path (jobs recorded before holds): deduct actual usage
		// only when this attempt inserted the ledger row.
		if err := debitUsageTx(ctx, tx, params.OrganizationID, params.AmountMicros); err != nil {
			return err
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin grant tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := GrantOn(ctx, tx, params); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit grant tx: %w", err)
	}
	return nil
}

// GrantOn writes a grant ledger row and updates org balances on an existing
// transaction (signup, webhook, etc.).
func GrantOn(ctx context.Context, db dbTX, params GrantParams) error {
	if params.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}
	if params.AmountMicros <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	result, err := db.ExecContext(
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
		return nil
	}

	_, err = db.ExecContext(
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
	LatencyMS          int
}

func (r *Repository) ListHistory(ctx context.Context, organizationID string) ([]HistoryRow, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT cl.id, cl.organization_id, cl.inference_request_id, cl.entry_type,
		        cl.amount_micros, cl.vendor_amount_micros, cl.idempotency_key, cl.note,
		        cl.created_at,
		        COALESCE(mc.name, ''),
		        COALESCE(ir.input_tokens, 0),
		        COALESCE(ir.output_tokens, 0),
		        COALESCE(ir.latency_ms, 0)
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
			&row.LatencyMS,
		); err != nil {
			return nil, err
		}
		entries = append(entries, row)
	}
	return entries, rows.Err()
}

type UsageDailyRow struct {
	Day          time.Time
	ModelID      string
	ModelName    string
	InputTokens  int64
	OutputTokens int64
	CostMicros   int64
}

type UsageRequestRow struct {
	ID           string
	CreatedAt    time.Time
	ModelName    string
	InputTokens  int
	OutputTokens int
	LatencyMS    int
	AmountMicros int64
	Status       string
}

func (r *Repository) SumUsageCost(ctx context.Context, organizationID string, from, to time.Time) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COALESCE(SUM(cl.amount_micros), 0)
		 FROM credit_ledger cl
		 WHERE cl.organization_id = $1
		   AND cl.entry_type = 'usage'
		   AND cl.created_at >= $2
		   AND cl.created_at < $3`,
		organizationID,
		from,
		to,
	).Scan(&total)
	return total, err
}

func (r *Repository) ListUsageDaily(ctx context.Context, organizationID string, from, to time.Time) ([]UsageDailyRow, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT (ir.created_at AT TIME ZONE 'UTC')::date AS day,
		        COALESCE(mc.id::text, ''),
		        COALESCE(NULLIF(mc.name, ''), 'Unknown'),
		        COALESCE(SUM(ir.input_tokens), 0),
		        COALESCE(SUM(ir.output_tokens), 0),
		        COALESCE(SUM(cl.amount_micros), 0)
		 FROM inference_requests ir
		 LEFT JOIN model_catalogue mc ON mc.id = ir.model_catalogue_id
		 LEFT JOIN credit_ledger cl ON cl.inference_request_id = ir.id AND cl.entry_type = 'usage'
		 WHERE ir.organization_id = $1
		   AND ir.created_at >= $2
		   AND ir.created_at < $3
		   AND ir.status = 'success'
		 GROUP BY 1, 2, 3
		 ORDER BY 1 ASC, 3 ASC`,
		organizationID,
		from,
		to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UsageDailyRow, 0)
	for rows.Next() {
		var row UsageDailyRow
		if err := rows.Scan(&row.Day, &row.ModelID, &row.ModelName, &row.InputTokens, &row.OutputTokens, &row.CostMicros); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) CountUsageRequests(ctx context.Context, organizationID string, from, to time.Time) (int, error) {
	var total int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM inference_requests ir
		 WHERE ir.organization_id = $1
		   AND ir.created_at >= $2
		   AND ir.created_at < $3`,
		organizationID,
		from,
		to,
	).Scan(&total)
	return total, err
}

func (r *Repository) ListUsageRequests(ctx context.Context, organizationID string, from, to time.Time, limit, offset int) ([]UsageRequestRow, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT ir.id, ir.created_at,
		        COALESCE(NULLIF(mc.name, ''), 'Unknown'),
		        ir.input_tokens, ir.output_tokens, ir.latency_ms,
		        COALESCE(cl.amount_micros, 0),
		        ir.status
		 FROM inference_requests ir
		 LEFT JOIN model_catalogue mc ON mc.id = ir.model_catalogue_id
		 LEFT JOIN credit_ledger cl ON cl.inference_request_id = ir.id AND cl.entry_type = 'usage'
		 WHERE ir.organization_id = $1
		   AND ir.created_at >= $2
		   AND ir.created_at < $3
		 ORDER BY ir.created_at DESC
		 LIMIT $4 OFFSET $5`,
		organizationID,
		from,
		to,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UsageRequestRow, 0)
	for rows.Next() {
		var row UsageRequestRow
		if err := rows.Scan(
			&row.ID,
			&row.CreatedAt,
			&row.ModelName,
			&row.InputTokens,
			&row.OutputTokens,
			&row.LatencyMS,
			&row.AmountMicros,
			&row.Status,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
