package organization

import (
	"context"
	"errors"
	"fmt"

	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
)

var ErrInvalid = errors.New("invalid request")

type Service struct {
	orgs *organizationRepository.Repository
}

func NewService(orgs *organizationRepository.Repository) *Service {
	return &Service{orgs: orgs}
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
	return &ListResponse{Organizations: orgs, Total: len(orgs)}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*CreateResponse, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, fmt.Errorf("missing user id")
	}

	org, err := s.orgs.CreateForUser(ctx, req.Name, req.Description, userID)
	if err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return &CreateResponse{Organization: *org}, nil
}
