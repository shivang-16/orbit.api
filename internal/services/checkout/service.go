package checkout

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	"github.com/shivang-16/orbit.api/internal/infra/dodo"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

var (
	ErrInvalid        = errors.New("invalid request")
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
	ErrPlanNotFound   = errors.New("plan not found")
)

type Service struct {
	dodo   *dodo.Client
	clerk  *clerk.Client
	plans  *planRepository.Repository
	orgs   *organizationRepository.Repository
	config config.Config
}

func NewService(
	dodoClient *dodo.Client,
	clerkClient *clerk.Client,
	plans *planRepository.Repository,
	orgs *organizationRepository.Repository,
	cfg config.Config,
) *Service {
	return &Service{dodo: dodoClient, clerk: clerkClient, plans: plans, orgs: orgs, config: cfg}
}

func (s *Service) CreateCheckout(ctx context.Context, req CreateCheckoutRequest) (*CreateCheckoutResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("missing user id")
	}

	orgID, err := s.orgIDForUser(ctx, userID, req.OrganizationID)
	if err != nil {
		return nil, err
	}

	plan, err := s.plans.GetBySlug(ctx, strings.TrimSpace(req.PlanSlug))
	if err != nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	profile, err := s.clerk.GetProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user profile: %w", err)
	}

	frontendURL := strings.TrimRight(s.config.FrontendURL, "/")
	params := dodo.CreateCheckoutSessionParams{
		ProductCart: []dodo.CartItem{{ProductID: plan.DodoProductID, Quantity: 1}},
		ReturnURL:   frontendURL + "/dashboard?checkout_success=1&plan=" + plan.Slug,
		Customer:    &dodo.Customer{Email: profile.Email},
		Metadata: map[string]string{
			"organization_id": orgID,
			"plan_id":         plan.ID,
			"plan_slug":       plan.Slug,
			"credits_micros":  strconv.FormatInt(plan.CreditsMicros, 10),
		},
	}
	if mandate := mandateMinAmountInrPaise(plan.PriceMicros); mandate > 0 {
		params.MandateMinAmountInrPaise = &mandate
	}

	session, err := s.dodo.CreateCheckoutSession(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create checkout session: %w", err)
	}

	return &CreateCheckoutResponse{CheckoutURL: session.CheckoutURL}, nil
}

func (s *Service) orgIDForUser(ctx context.Context, userID, organizationID string) (string, error) {
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
