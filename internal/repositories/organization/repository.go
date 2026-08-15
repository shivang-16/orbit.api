package organization

import (
	"context"
	"database/sql"

	"github.com/shivang-16/orbit.api/internal/model"
)

const DefaultOrganizationName = "Default Organization"

type dbTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Repository struct {
	db dbTX
}

func NewRepository(db dbTX) *Repository {
	return &Repository{db: db}
}

func (r *Repository) HasMembership(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(
		ctx,
		`SELECT EXISTS(
			SELECT 1 FROM organization_members WHERE user_id = $1
		)`,
		userID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) CreateDefaultForUser(ctx context.Context, userID string) (*model.Organization, error) {
	org := model.Organization{}
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO organizations (name, slug, created_by)
		 VALUES ($1, 'default-' || replace(gen_random_uuid()::text, '-', ''), $2)
		 RETURNING id, name, slug, created_by, created_at, updated_at`,
		DefaultOrganizationName,
		userID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, err
	}

	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO organization_members (organization_id, user_id, role)
		 VALUES ($1, $2, $3)`,
		org.ID,
		userID,
		model.OrgRoleAdmin,
	)
	if err != nil {
		return nil, err
	}

	return &org, nil
}
