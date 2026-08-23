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
	"github.com/shivang-16/orbit.api/internal/model"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

var (
	ErrInvalid        = errors.New("invalid request")
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
	ErrPlanNotFound   = errors.New("plan not found")
	ErrSamePlan       = errors.New("already on this plan")
	ErrDowngrade      = errors.New("downgrade not supported")
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

	if err := s.guardPlanChange(ctx, orgID, plan); err != nil {
		return nil, err
	}

	// Upgrades always go through hosted checkout so Dodo charges now and
	// the customer authorizes a new mandate at the higher plan amount.
	// In-place change-plan cannot raise an INR e-mandate, so Trial → Starter
	// would fail or wait until the next cycle. The old subscription is
	// cancelled when the new one becomes active.
	return s.createHostedCheckout(ctx, userID, orgID, plan)
}

func (s *Service) guardPlanChange(ctx context.Context, orgID string, target *model.Plan) error {
	org, err := s.orgs.GetByID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("load organization: %w", err)
	}
	if org == nil || strings.TrimSpace(org.PlanSlug) == "" {
		return nil
	}

	current, err := s.plans.GetBySlug(ctx, org.PlanSlug)
	if err != nil {
		return fmt.Errorf("load current plan: %w", err)
	}
	return planChangeError(current, target)
}

func (s *Service) createHostedCheckout(ctx context.Context, userID, orgID string, plan *model.Plan) (*CreateCheckoutResponse, error) {
	profile, err := s.clerk.GetProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user profile: %w", err)
	}

	frontendURL := strings.TrimRight(s.config.FrontendURL, "/")
	params := dodo.CreateCheckoutSessionParams{
		ProductCart: []dodo.CartItem{{ProductID: plan.DodoProductID, Quantity: 1}},
		ReturnURL:   frontendURL + "/billing/credits?checkout_success=1&plan=" + plan.Slug,
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

	return &CreateCheckoutResponse{CheckoutURL: session.CheckoutURL, PlanSlug: plan.Slug}, nil
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
