package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrInsufficientCredits = errors.New("insufficient credits")

type Hold struct {
	ID                    string
	AmountMicros          int64
	MaxTokens             int
	OrganizationID        string
	RemainingBeforeMicros int64
	RemainingAfterMicros  int64
}

// HoldPlanner computes the freeze from remaining credits while the org row
// is locked. Returning a non-nil error aborts the transaction (no hold).
type HoldPlanner func(available int64) (amountMicros int64, maxTokens int, err error)

func (r *Repository) PlaceHold(ctx context.Context, organizationID string, threshold int64, ttl time.Duration, plan HoldPlanner) (Hold, error) {
	if organizationID == "" {
		return Hold{}, fmt.Errorf("organization id is required")
	}
	if plan == nil {
		return Hold{}, fmt.Errorf("hold planner is required")
	}
	if ttl < time.Second {
		ttl = time.Minute
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Hold{}, fmt.Errorf("begin hold tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var remaining int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1 FOR UPDATE`,
		organizationID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return Hold{}, ErrInsufficientCredits
	}
	if err != nil {
		return Hold{}, fmt.Errorf("lock organization: %w", err)
	}

	if err := reclaimExpiredHoldsTx(ctx, tx, organizationID); err != nil {
		return Hold{}, err
	}

	err = tx.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1`,
		organizationID,
	).Scan(&remaining)
	if err != nil {
		return Hold{}, fmt.Errorf("read remaining: %w", err)
	}

	amount, maxTokens, err := plan(remaining)
	if err != nil {
		return Hold{}, err
	}
	if amount < 1 || maxTokens < 1 {
		return Hold{}, ErrInsufficientCredits
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE organizations
		 SET credits_remaining_micros = credits_remaining_micros - $1
		 WHERE id = $2
		   AND credits_remaining_micros >= $1
		   AND credits_remaining_micros >= $3`,
		amount,
		organizationID,
		threshold,
	)
	if err != nil {
		return Hold{}, fmt.Errorf("debit hold: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Hold{}, fmt.Errorf("debit hold rows: %w", err)
	}
	if updated == 0 {
		return Hold{}, ErrInsufficientCredits
	}

	var holdID string
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO credit_holds (organization_id, amount_micros, expires_at)
		 VALUES ($1, $2, now() + ($3 * INTERVAL '1 second'))
		 RETURNING id`,
		organizationID,
		amount,
		int64(ttl.Seconds()),
	).Scan(&holdID)
	if err != nil {
		return Hold{}, fmt.Errorf("insert hold: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Hold{}, fmt.Errorf("commit hold: %w", err)
	}
	return Hold{
		ID:                    holdID,
		AmountMicros:          amount,
		MaxTokens:             maxTokens,
		OrganizationID:        organizationID,
		RemainingBeforeMicros: remaining,
		RemainingAfterMicros:  remaining - amount,
	}, nil
}

func (r *Repository) ReleaseHold(ctx context.Context, holdID string) (SettleResult, error) {
	if holdID == "" {
		return SettleResult{}, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SettleResult{}, fmt.Errorf("begin release tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	settled, err := settleHoldTx(ctx, tx, holdID, 0)
	if err != nil {
		return SettleResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SettleResult{}, fmt.Errorf("commit release: %w", err)
	}
	return settled, nil
}

func (r *Repository) ReleaseExpiredHolds(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin expire tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT o.id
		 FROM organizations o
		 WHERE EXISTS (
			SELECT 1 FROM credit_holds h
			WHERE h.organization_id = o.id
			  AND h.settled_at IS NULL
			  AND h.expires_at < now()
		 )
		 FOR UPDATE OF o SKIP LOCKED`,
	)
	if err != nil {
		return 0, fmt.Errorf("lock orgs with expired holds: %w", err)
	}

	orgIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	released := 0
	for _, orgID := range orgIDs {
		n, err := reclaimExpiredHoldsTxCount(ctx, tx, orgID)
		if err != nil {
			return released, err
		}
		released += n
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit expire: %w", err)
	}
	return released, nil
}

func reclaimExpiredHoldsTx(ctx context.Context, tx dbTX, organizationID string) error {
	_, err := reclaimExpiredHoldsTxCount(ctx, tx, organizationID)
	return err
}

func reclaimExpiredHoldsTxCount(ctx context.Context, tx dbTX, organizationID string) (int, error) {
	var n int
	err := tx.QueryRowContext(
		ctx,
		`WITH expired AS (
			UPDATE credit_holds
			   SET settled_at = now()
			 WHERE organization_id = $1
			   AND settled_at IS NULL
			   AND expires_at < now()
			 RETURNING amount_micros
		 ), applied AS (
			UPDATE organizations
			   SET credits_remaining_micros = credits_remaining_micros
			       + COALESCE((SELECT SUM(amount_micros) FROM expired), 0)
			 WHERE id = $1
			 RETURNING 1
		 )
		 SELECT COALESCE((SELECT COUNT(*) FROM expired), 0)
		 FROM applied`,
		organizationID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("reclaim expired holds: %w", err)
	}
	return n, nil
}

type SettleResult struct {
	Found                 bool
	Applied               bool
	OrganizationID        string
	AmountMicros          int64
	ActualMicros          int64
	RefundMicros          int64
	RemainingBeforeMicros int64
	RemainingAfterMicros  int64
}

// settleHoldTx marks an open hold settled and applies the same actual spend
// as the credit_ledger usage row: remaining += hold - actual, used += actual.
// Net remaining is original - actual (unused freeze comes back; extra input
// beyond the estimate is taken from remaining so it stays in lockstep with
// the ledger). actualMicros is 0 on failure (full refund of the hold).
// A second call is a no-op so SQS retries cannot double-charge.
//
// Lock order is organization then hold, matching PlaceHold and the sweeper,
// so concurrent settle/reclaim cannot deadlock.
func settleHoldTx(ctx context.Context, tx dbTX, holdID string, actualMicros int64) (SettleResult, error) {
	if holdID == "" {
		return SettleResult{}, nil
	}
	if actualMicros < 0 {
		actualMicros = 0
	}

	var orgID string
	var amount int64
	err := tx.QueryRowContext(
		ctx,
		`SELECT organization_id, amount_micros FROM credit_holds WHERE id = $1`,
		holdID,
	).Scan(&orgID, &amount)
	if errors.Is(err, sql.ErrNoRows) {
		return SettleResult{}, nil
	}
	if err != nil {
		return SettleResult{}, fmt.Errorf("lookup hold: %w", err)
	}

	result := SettleResult{
		Found:          true,
		OrganizationID: orgID,
		AmountMicros:   amount,
		ActualMicros:   actualMicros,
	}

	var remaining int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1 FOR UPDATE`,
		orgID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return result, fmt.Errorf("organization missing for hold %s", holdID)
	}
	if err != nil {
		return result, fmt.Errorf("lock organization: %w", err)
	}

	var settledAt sql.NullTime
	err = tx.QueryRowContext(
		ctx,
		`SELECT amount_micros, settled_at FROM credit_holds WHERE id = $1 FOR UPDATE`,
		holdID,
	).Scan(&amount, &settledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SettleResult{}, nil
	}
	if err != nil {
		return result, fmt.Errorf("lock hold: %w", err)
	}
	result.AmountMicros = amount
	result.RemainingBeforeMicros = remaining
	result.ActualMicros = actualMicros
	if settledAt.Valid {
		return result, nil
	}

	exec, err := tx.ExecContext(
		ctx,
		`UPDATE credit_holds SET settled_at = now() WHERE id = $1 AND settled_at IS NULL`,
		holdID,
	)
	if err != nil {
		return result, fmt.Errorf("settle hold: %w", err)
	}
	updated, err := exec.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("settle hold rows: %w", err)
	}
	if updated == 0 {
		return result, nil
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE organizations
		    SET credits_remaining_micros = credits_remaining_micros + $1 - $2,
		        credits_used_micros = credits_used_micros + $2
		  WHERE id = $3`,
		amount,
		actualMicros,
		orgID,
	)
	if err != nil {
		return result, fmt.Errorf("apply hold settlement: %w", err)
	}
	result.Applied = true
	result.RefundMicros = amount - actualMicros
	result.RemainingAfterMicros = remaining + amount - actualMicros
	return result, nil
}

func debitUsageTx(ctx context.Context, tx dbTX, organizationID string, amount int64) error {
	if organizationID == "" || amount <= 0 {
		return nil
	}
	_, err := tx.ExecContext(
		ctx,
		`UPDATE organizations
		 SET credits_used_micros = credits_used_micros + $1,
		     credits_remaining_micros = credits_remaining_micros - $1
		 WHERE id = $2`,
		amount,
		organizationID,
	)
	if err != nil {
		return fmt.Errorf("update organization credits: %w", err)
	}
	return nil
}

func (r *Repository) Remaining(ctx context.Context, organizationID string) (remaining int64, ok bool, err error) {
	err = r.db.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1`,
		organizationID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return remaining, true, nil
}

func (r *Repository) ReclaimExpiredForOrg(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reclaim tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var remaining int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1 FOR UPDATE`,
		organizationID,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInsufficientCredits
	}
	if err != nil {
		return fmt.Errorf("lock organization: %w", err)
	}
	if err := reclaimExpiredHoldsTx(ctx, tx, organizationID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reclaim: %w", err)
	}
	return nil
}
