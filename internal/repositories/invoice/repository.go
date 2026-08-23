package invoice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shivang-16/orbit.api/internal/model"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

type UpsertParams struct {
	OrganizationID string
	PaymentID      string
	InvoiceID      string
	PlanSlug       string
	Amount         int
	Currency       string
	Status         string
	RefundStatus   string
	SubscriptionID string
	PaidAt         time.Time
}

func (r *Repository) Upsert(ctx context.Context, params UpsertParams) error {
	if strings.TrimSpace(params.OrganizationID) == "" || strings.TrimSpace(params.PaymentID) == "" {
		return fmt.Errorf("organization_id and payment_id are required")
	}
	paidAt := params.PaidAt
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = "succeeded"
	}

	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO invoices (
			organization_id, payment_id, invoice_id, plan_slug, amount, currency,
			status, refund_status, subscription_id, paid_at
		) VALUES ($1, $2, $3, $4, $5, COALESCE(NULLIF($6, ''), 'USD'), $7, $8, $9, $10)
		 ON CONFLICT (payment_id) DO UPDATE SET
			invoice_id = CASE WHEN EXCLUDED.invoice_id <> '' THEN EXCLUDED.invoice_id ELSE invoices.invoice_id END,
			plan_slug = CASE WHEN EXCLUDED.plan_slug <> '' THEN EXCLUDED.plan_slug ELSE invoices.plan_slug END,
			amount = CASE WHEN EXCLUDED.amount > 0 THEN EXCLUDED.amount ELSE invoices.amount END,
			currency = CASE WHEN EXCLUDED.currency <> '' THEN EXCLUDED.currency ELSE invoices.currency END,
			status = CASE WHEN EXCLUDED.status <> '' THEN EXCLUDED.status ELSE invoices.status END,
			refund_status = CASE WHEN EXCLUDED.refund_status <> '' THEN EXCLUDED.refund_status ELSE invoices.refund_status END,
			subscription_id = CASE WHEN EXCLUDED.subscription_id <> '' THEN EXCLUDED.subscription_id ELSE invoices.subscription_id END,
			paid_at = invoices.paid_at`,
		params.OrganizationID,
		strings.TrimSpace(params.PaymentID),
		strings.TrimSpace(params.InvoiceID),
		strings.TrimSpace(params.PlanSlug),
		params.Amount,
		currency,
		status,
		strings.TrimSpace(params.RefundStatus),
		strings.TrimSpace(params.SubscriptionID),
		paidAt,
	)
	if err != nil {
		return fmt.Errorf("upsert invoice: %w", err)
	}
	return nil
}

func (r *Repository) ListByOrg(ctx context.Context, organizationID string, limit, offset int) ([]model.Invoice, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, organization_id, payment_id, invoice_id, plan_slug, amount, currency,
		        status, refund_status, subscription_id, paid_at, created_at
		 FROM invoices
		 WHERE organization_id = $1
		 ORDER BY paid_at DESC
		 LIMIT $2 OFFSET $3`,
		organizationID,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list invoices: %w", err)
	}
	defer rows.Close()

	out := make([]model.Invoice, 0)
	for rows.Next() {
		var item model.Invoice
		if err := rows.Scan(
			&item.ID,
			&item.OrganizationID,
			&item.PaymentID,
			&item.InvoiceID,
			&item.PlanSlug,
			&item.Amount,
			&item.Currency,
			&item.Status,
			&item.RefundStatus,
			&item.SubscriptionID,
			&item.PaidAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) CountByOrg(ctx context.Context, organizationID string) (int, error) {
	var total int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM invoices WHERE organization_id = $1`,
		organizationID,
	).Scan(&total)
	return total, err
}

type SubscriptionRef struct {
	ID       string
	PlanSlug string
}

func (r *Repository) SubscriptionsForOrg(ctx context.Context, organizationID string) ([]SubscriptionRef, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return nil, nil
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT DISTINCT ON (subscription_id) subscription_id, plan_slug
		 FROM invoices
		 WHERE organization_id = $1 AND subscription_id <> ''
		 ORDER BY subscription_id, paid_at DESC`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SubscriptionRef, 0)
	for rows.Next() {
		var ref SubscriptionRef
		if err := rows.Scan(&ref.ID, &ref.PlanSlug); err != nil {
			return nil, err
		}
		ref.ID = strings.TrimSpace(ref.ID)
		ref.PlanSlug = strings.TrimSpace(ref.PlanSlug)
		if ref.ID != "" {
			out = append(out, ref)
		}
	}
	return out, rows.Err()
}

func (r *Repository) SubscriptionIDsForOrg(ctx context.Context, organizationID string) ([]string, error) {
	refs, err := r.SubscriptionsForOrg(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.ID)
	}
	return out, nil
}

func (r *Repository) LatestSubscriptionID(ctx context.Context, organizationID string) (string, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return "", nil
	}
	var subscriptionID string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT subscription_id FROM invoices
		 WHERE organization_id = $1 AND subscription_id <> ''
		 ORDER BY paid_at DESC
		 LIMIT 1`,
		organizationID,
	).Scan(&subscriptionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(subscriptionID), nil
}

func (r *Repository) GetByPayment(ctx context.Context, paymentID string) (*model.Invoice, error) {
	return r.scanOne(ctx,
		`SELECT id, organization_id, payment_id, invoice_id, plan_slug, amount, currency,
		        status, refund_status, subscription_id, paid_at, created_at
		 FROM invoices
		 WHERE payment_id = $1`,
		paymentID,
	)
}

func (r *Repository) GetByPaymentID(ctx context.Context, organizationID, paymentID string) (*model.Invoice, error) {
	return r.scanOne(ctx,
		`SELECT id, organization_id, payment_id, invoice_id, plan_slug, amount, currency,
		        status, refund_status, subscription_id, paid_at, created_at
		 FROM invoices
		 WHERE organization_id = $1 AND payment_id = $2`,
		organizationID,
		paymentID,
	)
}

func (r *Repository) scanOne(ctx context.Context, query string, args ...any) (*model.Invoice, error) {
	var item model.Invoice
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.PaymentID,
		&item.InvoiceID,
		&item.PlanSlug,
		&item.Amount,
		&item.Currency,
		&item.Status,
		&item.RefundStatus,
		&item.SubscriptionID,
		&item.PaidAt,
		&item.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) UpdateRefundStatus(ctx context.Context, paymentID, refundStatus string) (bool, error) {
	paymentID = strings.TrimSpace(paymentID)
	refundStatus = strings.TrimSpace(refundStatus)
	if paymentID == "" || refundStatus == "" {
		return false, nil
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE invoices SET refund_status = $2 WHERE payment_id = $1`,
		paymentID,
		refundStatus,
	)
	if err != nil {
		return false, fmt.Errorf("update invoice refund: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
