package credits

import (
	"context"
	"errors"
	"fmt"
	"strings"

	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
)

var (
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
)

type Service struct {
	billing *billingRepository.Repository
	orgs    *organizationRepository.Repository
}

func NewService(
	billing *billingRepository.Repository,
	orgs *organizationRepository.Repository,
) *Service {
	return &Service{billing: billing, orgs: orgs}
}

func (s *Service) GetOrganizationCredits(ctx context.Context, organizationID string) (*OrganizationCreditsResponse, error) {
	orgID, err := s.orgIDForUser(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("get organization credits: %w", err)
	}
	if org == nil {
		return nil, ErrNoOrganization
	}

	return &OrganizationCreditsResponse{
		OrganizationID:         org.ID,
		OrganizationName:       org.Name,
		CreditsGrantedMicros:   org.CreditsGrantedMicros,
		CreditsUsedMicros:      org.CreditsUsedMicros,
		CreditsRemainingMicros: org.CreditsRemainingMicros,
	}, nil
}

func (s *Service) ListOrganizationCreditHistory(ctx context.Context, organizationID string) (*HistoryResponse, error) {
	orgID, err := s.orgIDForUser(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	rows, err := s.billing.ListHistory(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list credit history: %w", err)
	}

	entries := make([]HistoryEntry, 0, len(rows))
	for _, row := range rows {
		amount := row.AmountMicros
		if row.EntryType == "usage" {
			amount = -amount
		}
		entries = append(entries, HistoryEntry{
			ID:             row.ID,
			EntryType:      row.EntryType,
			TypeLabel:      typeLabel(row.EntryType),
			Description:    historyDescription(row),
			AmountMicros:   amount,
			IdempotencyKey: row.IdempotencyKey,
			CreatedAt:      row.CreatedAt,
		})
	}

	return &HistoryResponse{Entries: entries, Total: len(entries)}, nil
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

func typeLabel(entryType string) string {
	switch entryType {
	case "grant":
		return "Credit added"
	case "usage":
		return "Usage"
	case "refund":
		return "Refund"
	case "adjustment":
		return "Adjustment"
	default:
		return entryType
	}
}

func historyDescription(row billingRepository.HistoryRow) string {
	switch row.EntryType {
	case "grant":
		if slug, ok := strings.CutPrefix(row.Note, "plan:"); ok && slug != "" {
			return titleCase(slug) + " plan purchase"
		}
		if row.Note != "" {
			return row.Note
		}
		return "Manual credit addition"
	case "usage":
		if row.ModelName != "" {
			return fmt.Sprintf("%s — %d in / %d out", row.ModelName, row.InputTokens, row.OutputTokens)
		}
		return "Inference usage"
	case "refund":
		if row.Note != "" {
			return row.Note
		}
		return "Credit refund"
	case "adjustment":
		if row.Note != "" {
			return row.Note
		}
		return "Balance adjustment"
	default:
		if row.Note != "" {
			return row.Note
		}
		return row.EntryType
	}
}

func titleCase(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + strings.ToLower(value[1:])
}
