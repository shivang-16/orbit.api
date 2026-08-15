package user

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
)

type Service struct {
	db    *sql.DB
	users *userRepository.Repository
	orgs  *organizationRepository.Repository
	clerk *clerk.Client
}

func NewService(
	db *sql.DB,
	users *userRepository.Repository,
	orgs *organizationRepository.Repository,
	clerkClient *clerk.Client,
) *Service {
	return &Service{db: db, users: users, orgs: orgs, clerk: clerkClient}
}

func (s *Service) Sync(ctx context.Context) (*model.User, bool, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, false, fmt.Errorf("missing user id")
	}

	existing, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("get user: %w", err)
	}
	if existing != nil {
		if err := s.ensureDefaultOrg(ctx, existing.ID); err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}

	profile, err := s.clerk.GetProfile(ctx, userID)
	if err != nil {
		return nil, false, err
	}

	created, err := s.createUserWithDefaultOrg(ctx, &model.User{
		ID:       profile.ID,
		Email:    profile.Email,
		Name:     profile.Name,
		ImageURL: profile.ImageURL,
	})
	if err != nil {
		return nil, false, err
	}

	return created, true, nil
}

func (s *Service) createUserWithDefaultOrg(ctx context.Context, user *model.User) (*model.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	created, err := userRepository.NewRepository(tx).Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if _, err := organizationRepository.NewRepository(tx).CreateDefaultForUser(ctx, created.ID); err != nil {
		return nil, fmt.Errorf("create default organization: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return created, nil
}

func (s *Service) ensureDefaultOrg(ctx context.Context, userID string) error {
	has, err := s.orgs.HasMembership(ctx, userID)
	if err != nil {
		return fmt.Errorf("check organization: %w", err)
	}
	if has {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	orgs := organizationRepository.NewRepository(tx)
	has, err = orgs.HasMembership(ctx, userID)
	if err != nil {
		return fmt.Errorf("check organization: %w", err)
	}
	if has {
		return tx.Commit()
	}

	if _, err := orgs.CreateDefaultForUser(ctx, userID); err != nil {
		return fmt.Errorf("create default organization: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
