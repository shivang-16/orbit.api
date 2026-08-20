package apikey

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
)

var (
	ErrInvalid        = errors.New("invalid request")
	ErrNoOrganization = errors.New("no organization")
	ErrForbidden      = errors.New("forbidden")
	ErrAdminRequired  = errors.New("admin required")
	ErrNotFound       = errors.New("not found")
)

type Service struct {
	keys *apikeyRepository.Repository
	orgs *organizationRepository.Repository
}

func NewService(
	keys *apikeyRepository.Repository,
	orgs *organizationRepository.Repository,
) *Service {
	return &Service{keys: keys, orgs: orgs}
}

func (s *Service) List(ctx context.Context, organizationID string) (*ListResponse, error) {
	orgID, role, err := s.orgAccessForUser(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	keys, err := s.keys.ListByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	return &ListResponse{
		Keys:      keys,
		Total:     len(keys),
		Role:      role,
		CanDelete: role == model.OrgRoleAdmin,
	}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*CreateResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("missing user id")
	}

	orgID, _, err := s.orgAccessForUser(ctx, req.OrganizationID)
	if err != nil {
		return nil, err
	}

	secret, preview, hash, err := generateSecret()
	if err != nil {
		return nil, err
	}

	item, err := s.keys.Create(ctx, apikeyRepository.CreateParams{
		OrganizationID: orgID,
		CreatedBy:      userID,
		Name:           req.Name,
		KeyHash:        hash,
		KeyPreview:     preview,
		ExpiresAt:      req.expiresAt(time.Now().UTC()),
	})
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}

	return &CreateResponse{Key: *item, Secret: secret}, nil
}

func (s *Service) Delete(ctx context.Context, id, organizationID string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrNotFound
	}

	orgID, role, err := s.orgAccessForUser(ctx, organizationID)
	if err != nil {
		return err
	}
	if role != model.OrgRoleAdmin {
		return ErrAdminRequired
	}

	ok, err := s.keys.Deactivate(ctx, id, orgID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *Service) orgAccessForUser(ctx context.Context, organizationID string) (string, model.OrgRole, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return "", "", fmt.Errorf("missing user id")
	}

	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		org, err := s.orgs.GetFirstForUser(ctx, userID)
		if err != nil {
			return "", "", fmt.Errorf("get organization: %w", err)
		}
		if org == nil {
			return "", "", ErrNoOrganization
		}
		organizationID = org.ID
	}

	role, member, err := s.orgs.GetRole(ctx, userID, organizationID)
	if err != nil {
		return "", "", fmt.Errorf("check organization: %w", err)
	}
	if !member {
		return "", "", ErrForbidden
	}
	return organizationID, role, nil
}
