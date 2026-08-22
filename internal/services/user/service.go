package user

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	"github.com/shivang-16/orbit.api/internal/logger"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
	mailService "github.com/shivang-16/orbit.api/internal/services/mail"
)

type Service struct {
	db            *sql.DB
	users         *userRepository.Repository
	orgs          *organizationRepository.Repository
	clerk         *clerk.Client
	mail          *mailService.Service
	signupCredits int64
}

func NewService(
	db *sql.DB,
	users *userRepository.Repository,
	orgs *organizationRepository.Repository,
	clerkClient *clerk.Client,
	mail *mailService.Service,
	signupCredits int64,
) *Service {
	return &Service{db: db, users: users, orgs: orgs, clerk: clerkClient, mail: mail, signupCredits: signupCredits}
}

func (s *Service) Sync(ctx context.Context) (*model.User, bool, error) {
	userID, ok := authMiddleware.UserID(ctx)
	if !ok {
		return nil, false, fmt.Errorf("missing user id")
	}

	existing, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("get user %s: %w", userID, err)
	}
	if existing != nil {
		if err := s.ensureDefaultOrg(ctx, existing.ID); err != nil {
			return nil, false, fmt.Errorf("ensure default org for %s: %w", userID, err)
		}
		return existing, false, nil
	}

	logger.Info(ctx, "users/sync: clerk GetProfile", "user_id", userID)
	profile, err := s.clerk.GetProfile(ctx, userID)
	if err != nil {
		return nil, false, fmt.Errorf("clerk GetProfile user=%s: %w", userID, err)
	}

	created, err := s.createUserWithDefaultOrg(ctx, &model.User{
		ID:       profile.ID,
		Email:    profile.Email,
		Name:     profile.Name,
		ImageURL: profile.ImageURL,
	})
	if err != nil {
		return nil, false, fmt.Errorf("create user %s: %w", userID, err)
	}

	s.sendWelcome(created)
	return created, true, nil
}

func (s *Service) sendWelcome(user *model.User) {
	if s.mail == nil || user == nil {
		return
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error(context.Background(), "mail: welcome email panicked", "recover", rec)
			}
		}()
		ctx := logger.SetTag(context.Background(), logger.TagMail)
		ctx = logger.SetUser(ctx, user.ID, user.Email)
		s.mail.SendWelcome(ctx, user.Email, firstName(user.Name))
	}()
}

func firstName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if i := strings.IndexByte(name, ' '); i > 0 {
		return name[:i]
	}
	return name
}

func (s *Service) createUserWithDefaultOrg(ctx context.Context, user *model.User) (*model.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	orgs := organizationRepository.NewRepository(tx)
	if err := orgs.LockCreator(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("lock creator: %w", err)
	}

	created, err := userRepository.NewRepository(tx).Create(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	org, err := orgs.CreateDefaultForUser(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf("create default organization: %w", err)
	}
	if err := grantSignupCredits(ctx, tx, created.ID, org.ID, s.signupCredits); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	logger.Info(ctx, "users/sync: granted signup credits",
		"user_id", created.ID,
		"org_id", org.ID,
		"amount_micros", s.signupCredits,
	)

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
	if err := orgs.LockCreator(ctx, userID); err != nil {
		return fmt.Errorf("lock creator: %w", err)
	}

	has, err = orgs.HasMembership(ctx, userID)
	if err != nil {
		return fmt.Errorf("check organization: %w", err)
	}
	if has {
		return tx.Commit()
	}

	org, err := orgs.CreateDefaultForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("create default organization: %w", err)
	}
	if err := grantSignupCredits(ctx, tx, userID, org.ID, s.signupCredits); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func grantSignupCredits(ctx context.Context, tx *sql.Tx, userID, organizationID string, amountMicros int64) error {
	if err := billingRepository.GrantOn(ctx, tx, billingRepository.GrantParams{
		OrganizationID: organizationID,
		AmountMicros:   amountMicros,
		IdempotencyKey: "signup-credits:" + userID,
		Note:           "Welcome credits",
	}); err != nil {
		return fmt.Errorf("grant signup credits: %w", err)
	}
	return nil
}
