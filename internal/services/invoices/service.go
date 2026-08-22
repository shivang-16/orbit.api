package invoices

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/dodo"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	invoiceRepository "github.com/shivang-16/orbit.api/internal/repositories/invoice"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

const (
	defaultLimit = 25
	maxLimit     = 75
)

var (
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
	ErrInvalidPage    = errors.New("invalid page")
	ErrNotFound       = errors.New("invoice not found")
)

type Service struct {
	dodo     *dodo.Client
	invoices *invoiceRepository.Repository
	orgs     *organizationRepository.Repository
	plans    *planRepository.Repository
}

func NewService(
	dodoClient *dodo.Client,
	invoices *invoiceRepository.Repository,
	orgs *organizationRepository.Repository,
	plans *planRepository.Repository,
) *Service {
	return &Service{dodo: dodoClient, invoices: invoices, orgs: orgs, plans: plans}
}

type Invoice struct {
	PaymentID      string    `json:"payment_id"`
	InvoiceID      string    `json:"invoice_id"`
	PlanName       string    `json:"plan_name"`
	PlanSlug       string    `json:"plan_slug"`
	Amount         int       `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	StatusLabel    string    `json:"status_label"`
	RefundStatus   string    `json:"refund_status"`
	Downloadable   bool      `json:"downloadable"`
	SubscriptionID string    `json:"subscription_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type ListResponse struct {
	Invoices []Invoice `json:"invoices"`
	Page     int       `json:"page"`
	Limit    int       `json:"limit"`
	Total    int       `json:"total"`
}

func (s *Service) List(ctx context.Context, organizationID, pageRaw, limitRaw string) (*ListResponse, error) {
	orgID, err := s.orgIDForUser(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	page, limit, err := parsePage(pageRaw, limitRaw)
	if err != nil {
		return nil, err
	}

	total, err := s.invoices.CountByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("count invoices: %w", err)
	}
	rows, err := s.invoices.ListByOrg(ctx, orgID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}

	out := make([]Invoice, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.toInvoice(ctx, row))
	}
	return &ListResponse{Invoices: out, Page: page, Limit: limit, Total: total}, nil
}

func (s *Service) InvoicePDF(ctx context.Context, organizationID, paymentID string) ([]byte, string, error) {
	orgID, err := s.orgIDForUser(ctx, organizationID)
	if err != nil {
		return nil, "", err
	}
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return nil, "", ErrNotFound
	}

	row, err := s.invoices.GetByPaymentID(ctx, orgID, paymentID)
	if err != nil {
		return nil, "", fmt.Errorf("load invoice: %w", err)
	}
	if row == nil {
		return nil, "", ErrNotFound
	}

	body, contentType, err := s.dodo.GetInvoicePDF(ctx, paymentID)
	if err != nil {
		return nil, "", fmt.Errorf("download invoice pdf: %w", err)
	}
	if contentType == "" {
		contentType = "application/pdf"
	}
	return body, contentType, nil
}

func (s *Service) toInvoice(ctx context.Context, row model.Invoice) Invoice {
	planName := planDisplayName(row.PlanSlug)
	if s.plans != nil && row.PlanSlug != "" {
		if plan, err := s.plans.GetBySlug(ctx, row.PlanSlug); err == nil && plan != nil && plan.Name != "" {
			planName = plan.Name
		}
	}
	invoiceID := strings.TrimSpace(row.InvoiceID)
	if invoiceID == "" {
		invoiceID = row.PaymentID
	}
	return Invoice{
		PaymentID:      row.PaymentID,
		InvoiceID:      invoiceID,
		PlanName:       planName,
		PlanSlug:       row.PlanSlug,
		Amount:         row.Amount,
		Currency:       row.Currency,
		Status:         row.Status,
		StatusLabel:    statusLabel(row.Status, row.RefundStatus),
		RefundStatus:   row.RefundStatus,
		Downloadable:   strings.EqualFold(row.Status, "succeeded"),
		SubscriptionID: row.SubscriptionID,
		CreatedAt:      row.PaidAt,
	}
}

func (s *Service) orgIDForUser(ctx context.Context, organizationID string) (string, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return "", fmt.Errorf("missing user id")
	}

	organizationID = strings.TrimSpace(organizationID)
	if organizationID != "" {
		member, err := s.orgs.IsMember(ctx, userID, organizationID)
		if err != nil {
			return "", fmt.Errorf("check organization: %w", err)
		}
		if !member {
			return "", ErrForbidden
		}
		return organizationID, nil
	}

	org, err := s.orgs.GetFirstForUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get organization: %w", err)
	}
	if org == nil {
		return "", ErrNoOrganization
	}
	return org.ID, nil
}

func parsePage(pageRaw, limitRaw string) (page, limit int, err error) {
	page = 1
	limit = defaultLimit
	if strings.TrimSpace(pageRaw) != "" {
		page, err = strconv.Atoi(pageRaw)
		if err != nil || page < 1 {
			return 0, 0, ErrInvalidPage
		}
	}
	if strings.TrimSpace(limitRaw) != "" {
		limit, err = strconv.Atoi(limitRaw)
		if err != nil || limit < 1 {
			return 0, 0, ErrInvalidPage
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit, nil
}

func statusLabel(status, refund string) string {
	switch strings.ToLower(strings.TrimSpace(refund)) {
	case "full":
		return "Refunded"
	case "partial":
		return "Partially refunded"
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded":
		return "Paid"
	case "failed":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	case "partially_captured", "partially_captured_and_capturable":
		return "Partially paid"
	case "processing", "requires_customer_action", "requires_merchant_action", "requires_payment_method", "requires_confirmation", "requires_capture":
		return "Processing"
	case "":
		return "Unknown"
	default:
		return status
	}
}

func planDisplayName(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "Plan purchase"
	}
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}
