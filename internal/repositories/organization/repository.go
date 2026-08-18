package organization

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/shivang-16/orbit.api/internal/model"
)

const DefaultOrganizationName = "Default Organization"

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type dbTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
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

func (r *Repository) IsMember(ctx context.Context, userID, organizationID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(
		ctx,
		`SELECT EXISTS(
			SELECT 1 FROM organization_members
			WHERE user_id = $1 AND organization_id = $2
		)`,
		userID,
		organizationID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) CreateDefaultForUser(ctx context.Context, userID string) (*model.Organization, error) {
	return r.create(ctx, DefaultOrganizationName, "", userID, "default-"+randomSuffix())
}

func (r *Repository) CreateForUser(ctx context.Context, name, description, userID string) (*model.Organization, error) {
	return r.create(ctx, name, description, userID, uniqueSlug(name))
}

func (r *Repository) create(ctx context.Context, name, description, userID, slug string) (*model.Organization, error) {
	org := model.Organization{}
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO organizations (name, slug, description, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, slug, description, created_by, created_at, updated_at`,
		name,
		slug,
		description,
		userID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
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

func (r *Repository) ListForUser(ctx context.Context, userID string) ([]model.Organization, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT o.id, o.name, o.slug, o.description, o.created_by, o.created_at, o.updated_at
		 FROM organizations o
		 INNER JOIN organization_members m ON m.organization_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := make([]model.Organization, 0)
	for rows.Next() {
		var org model.Organization
		if err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Slug,
			&org.Description,
			&org.CreatedBy,
			&org.CreatedAt,
			&org.UpdatedAt,
		); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.Organization, error) {
	org := model.Organization{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, name, slug, description, created_by,
		        credits_granted_micros, credits_used_micros, credits_remaining_micros,
		        created_at, updated_at
		 FROM organizations
		 WHERE id = $1`,
		id,
	).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.CreatedBy,
		&org.CreditsGrantedMicros,
		&org.CreditsUsedMicros,
		&org.CreditsRemainingMicros,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetCreditsRemaining is a lightweight read used on the hot inference path
// to gate requests on balance without loading the full organization row.
// ok is false if the organization does not exist.
func (r *Repository) GetCreditsRemaining(ctx context.Context, id string) (remaining int64, ok bool, err error) {
	err = r.db.QueryRowContext(
		ctx,
		`SELECT credits_remaining_micros FROM organizations WHERE id = $1`,
		id,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return remaining, true, nil
}

func (r *Repository) GetFirstForUser(ctx context.Context, userID string) (*model.Organization, error) {
	org := model.Organization{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT o.id, o.name, o.slug, o.description, o.created_by, o.created_at, o.updated_at
		 FROM organizations o
		 INNER JOIN organization_members m ON m.organization_id = o.id
		 WHERE m.user_id = $1
		 ORDER BY o.created_at ASC
		 LIMIT 1`,
		userID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &org.CreatedBy, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func uniqueSlug(name string) string {
	base := slugify(name)
	if base == "" {
		base = "org"
	}
	return base + "-" + randomSuffix()
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugCleaner.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func randomSuffix() string {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte("orbit"))
	}
	return hex.EncodeToString(raw)
}
