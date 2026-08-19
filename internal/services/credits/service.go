package credits

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
			ID:           row.ID,
			EntryType:    row.EntryType,
			TypeLabel:    typeLabel(row.EntryType),
			ModelName:    historyModelName(row),
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			LatencyMS:    row.LatencyMS,
			AmountMicros: amount,
			CreatedAt:    row.CreatedAt,
		})
	}

	return &HistoryResponse{Entries: entries, Total: len(entries)}, nil
}

func (s *Service) GetUsage(ctx context.Context, organizationID, preset, pageRaw, limitRaw string) (*UsageResponse, error) {
	orgID, err := s.orgIDForUser(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	window, err := parseUsageRange(preset, time.Now())
	if err != nil {
		return nil, err
	}
	page, limit, err := parseUsagePage(pageRaw, limitRaw)
	if err != nil {
		return nil, err
	}

	daily, err := s.billing.ListUsageDaily(ctx, orgID, window.From, window.To)
	if err != nil {
		return nil, fmt.Errorf("list usage daily: %w", err)
	}
	total, err := s.billing.CountUsageRequests(ctx, orgID, window.From, window.To)
	if err != nil {
		return nil, fmt.Errorf("count usage requests: %w", err)
	}
	requests, err := s.billing.ListUsageRequests(ctx, orgID, window.From, window.To, limit, (page-1)*limit)
	if err != nil {
		return nil, fmt.Errorf("list usage requests: %w", err)
	}
	costMicros, err := s.billing.SumUsageCost(ctx, orgID, window.From, window.To)
	if err != nil {
		return nil, fmt.Errorf("sum usage cost: %w", err)
	}

	byDay := map[string][]UsageModelPoint{}
	var inputTotal, outputTotal int64
	for _, row := range daily {
		day := row.Day.UTC().Format("2006-01-02")
		total := row.InputTokens + row.OutputTokens
		inputTotal += row.InputTokens
		outputTotal += row.OutputTokens
		byDay[day] = append(byDay[day], UsageModelPoint{
			ModelID:      row.ModelID,
			ModelName:    row.ModelName,
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			TotalTokens:  total,
		})
	}

	series := make([]UsageDay, 0)
	for _, day := range eachUTCDay(window.From, window.To) {
		key := day.Format("2006-01-02")
		models := byDay[key]
		if models == nil {
			models = []UsageModelPoint{}
		}
		series = append(series, UsageDay{Date: key, Models: models})
	}

	outRequests := make([]UsageRequest, 0, len(requests))
	for _, row := range requests {
		outRequests = append(outRequests, UsageRequest{
			ID:           row.ID,
			CreatedAt:    row.CreatedAt,
			ModelName:    row.ModelName,
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			LatencyMS:    row.LatencyMS,
			AmountMicros: -row.AmountMicros,
			Status:       row.Status,
		})
	}

	return &UsageResponse{
		Range:         window.Preset,
		From:          window.From,
		To:            window.To,
		InputTokens:   inputTotal,
		OutputTokens:  outputTotal,
		TotalTokens:   inputTotal + outputTotal,
		CostMicros:    costMicros,
		Series:        series,
		Requests:      outRequests,
		RequestsPage:  page,
		RequestsLimit: limit,
		RequestsTotal: total,
	}, nil
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

func historyModelName(row billingRepository.HistoryRow) string {
	if row.EntryType == "usage" {
		if row.ModelName != "" {
			return row.ModelName
		}
		return "Inference"
	}
	return historyDescription(row)
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
