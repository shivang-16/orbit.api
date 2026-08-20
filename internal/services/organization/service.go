package organization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
)

var (
	ErrInvalid        = errors.New("invalid request")
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
	ErrOrgLimit       = errors.New("organization limit reached")
	ErrMemberLimit    = errors.New("member limit reached")
	ErrUserNotFound   = errors.New("user not found")
	ErrAlreadyMember  = errors.New("already a member")
)

type Service struct {
	db    *sql.DB
	orgs  *organizationRepository.Repository
	users *userRepository.Repository
}

func NewService(
	db *sql.DB,
	orgs *organizationRepository.Repository,
	users *userRepository.Repository,
) *Service {
	return &Service{db: db, orgs: orgs, users: users}
}

func (s *Service) List(ctx context.Context) (*ListResponse, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("missing user id")
	}

	orgs, err := s.orgs.ListForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	entitlement, err := s.entitlementForUser(ctx, s.orgs, userID)
	if err != nil {
		return nil, err
	}
	return &ListResponse{Organizations: orgs, Total: len(orgs), Entitlement: entitlement}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*CreateResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("missing user id")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	orgs := organizationRepository.NewRepository(tx)
	if err := orgs.LockCreator(ctx, userID); err != nil {
		return nil, fmt.Errorf("lock creator: %w", err)
	}

	entitlement, err := s.entitlementForUser(ctx, orgs, userID)
	if err != nil {
		return nil, err
	}
	if !entitlement.CanCreateOrganization {
		return nil, ErrOrgLimit
	}

	org, err := orgs.CreateForUser(ctx, req.Name, req.Description, userID)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	entitlement.OrganizationCount++
	entitlement.CanCreateOrganization = entitlement.UnlimitedOrganizations ||
		(entitlement.MaxOrganizations != nil && entitlement.OrganizationCount < *entitlement.MaxOrganizations)

	return &CreateResponse{Organization: *org, Entitlement: entitlement}, nil
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*UpdateResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	org, role, err := s.orgForMember(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	if role != model.OrgRoleAdmin {
		return nil, ErrForbidden
	}

	updated, err := s.orgs.Update(ctx, org.ID, req.Name, req.Description)
	if err != nil {
		return nil, fmt.Errorf("update organization: %w", err)
	}
	if updated == nil {
		return nil, ErrNoOrganization
	}
	return &UpdateResponse{Organization: *updated}, nil
}

func (s *Service) ListMembers(ctx context.Context, organizationID string) (*ListMembersResponse, error) {
	org, role, err := s.orgForMember(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	members, err := s.orgs.ListMembers(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	entitlement, err := s.entitlementForUser(ctx, s.orgs, org.CreatedBy)
	if err != nil {
		return nil, err
	}

	return &ListMembersResponse{
		Members:     members,
		Total:       len(members),
		Role:        role,
		Entitlement: entitlement.memberEntitlement(len(members)),
	}, nil
}

func (s *Service) AddMember(ctx context.Context, req AddMemberRequest) (*AddMemberResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	org, role, err := s.orgForMember(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}
	if role != model.OrgRoleAdmin {
		return nil, ErrForbidden
	}

	invitee, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if invitee == nil {
		return nil, ErrUserNotFound
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	orgs := organizationRepository.NewRepository(tx)
	if err := orgs.LockMembers(ctx, org.ID); err != nil {
		return nil, fmt.Errorf("lock members: %w", err)
	}

	entitlement, err := s.entitlementForUser(ctx, orgs, org.CreatedBy)
	if err != nil {
		return nil, err
	}
	count, err := orgs.CountMembers(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("count members: %w", err)
	}
	if !entitlement.canAddMember(count) {
		return nil, ErrMemberLimit
	}

	member, err := orgs.AddMember(ctx, org.ID, invitee.ID, model.OrgRole(req.Role))
	if err != nil {
		if errors.Is(err, organizationRepository.ErrAlreadyMember) {
			return nil, ErrAlreadyMember
		}
		return nil, fmt.Errorf("add member: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &AddMemberResponse{
		Member:      *member,
		Entitlement: entitlement.memberEntitlement(count + 1),
	}, nil
}

func (s *Service) entitlementForUser(
	ctx context.Context,
	orgs *organizationRepository.Repository,
	userID string,
) (Entitlement, error) {
	count, err := orgs.CountCreatedBy(ctx, userID)
	if err != nil {
		return Entitlement{}, fmt.Errorf("count organizations: %w", err)
	}
	plan, err := orgs.HighestPlanForCreator(ctx, userID)
	if err != nil {
		return Entitlement{}, fmt.Errorf("load plan limits: %w", err)
	}
	if plan == nil {
		return entitlementFor("", count, nil, nil), nil
	}
	return entitlementFor(plan.Slug, count, plan.MaxOrganizations, plan.MaxMembersPerOrg), nil
}

func (s *Service) orgForMember(ctx context.Context, organizationID string) (*model.Organization, model.OrgRole, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, "", fmt.Errorf("missing user id")
	}

	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		org, err := s.orgs.GetFirstForUser(ctx, userID)
		if err != nil {
			return nil, "", fmt.Errorf("get organization: %w", err)
		}
		if org == nil {
			return nil, "", ErrNoOrganization
		}
		organizationID = org.ID
	}

	role, member, err := s.orgs.GetRole(ctx, userID, organizationID)
	if err != nil {
		return nil, "", fmt.Errorf("check membership: %w", err)
	}
	if !member {
		return nil, "", ErrForbidden
	}

	org, err := s.orgs.GetByID(ctx, organizationID)
	if err != nil {
		return nil, "", fmt.Errorf("get organization: %w", err)
	}
	if org == nil {
		return nil, "", ErrNoOrganization
	}
	return org, role, nil
}
