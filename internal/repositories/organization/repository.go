package organization

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shivang-16/orbit.api/internal/model"
)

const DefaultOrganizationName = "Default Organization"

const listForUserOrder = `
		 ORDER BY
		   CASE
		     WHEN o.name = '` + DefaultOrganizationName + `' THEN 0
		     WHEN o.slug ~ '^default-[0-9a-f]{8}$' THEN 1
		     ELSE 2
		   END,
		   o.created_at ASC`

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

type CreatorPlan struct {
	Slug             string
	MaxOrganizations *int
	MaxMembersPerOrg *int
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

func (r *Repository) LockCreator(ctx context.Context, userID string) error {
	// Transaction-scoped. Hold this before counting or inserting orgs for
	// this user so /users/sync default-org creation and POST /organizations
	// cannot both commit when the count is still zero.
	_, err := r.db.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('org-create:' || $1))`,
		userID,
	)
	return err
}

func (r *Repository) LockMembers(ctx context.Context, organizationID string) error {
	_, err := r.db.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('org-members:' || $1))`,
		organizationID,
	)
	return err
}

func (r *Repository) CountCreatedBy(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM organizations WHERE created_by = $1`,
		userID,
	).Scan(&count)
	return count, err
}

func (r *Repository) HighestPlanForCreator(ctx context.Context, userID string) (*CreatorPlan, error) {
	var slug string
	var maxOrgs, maxMembers sql.NullInt64
	err := r.db.QueryRowContext(
		ctx,
		`SELECT p.slug, p.max_organizations, p.max_members_per_org
		 FROM organizations o
		 INNER JOIN plans p ON p.slug = o.plan_slug
		 WHERE o.created_by = $1
		 ORDER BY p.sort_order DESC
		 LIMIT 1`,
		userID,
	).Scan(&slug, &maxOrgs, &maxMembers)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &CreatorPlan{
		Slug:             slug,
		MaxOrganizations: nullIntPtr(maxOrgs),
		MaxMembersPerOrg: nullIntPtr(maxMembers),
	}, nil
}

func (r *Repository) SetPlanSlugIfHigher(ctx context.Context, organizationID, planSlug string) error {
	var incomingOrder int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT sort_order FROM plans WHERE slug = $1`,
		planSlug,
	).Scan(&incomingOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("unknown plan %q", planSlug)
	}
	if err != nil {
		return err
	}

	result, err := r.db.ExecContext(
		ctx,
		`UPDATE organizations
		 SET plan_slug = $2
		 WHERE id = $1
		   AND COALESCE(
		     (SELECT p.sort_order FROM plans p WHERE p.slug = organizations.plan_slug),
		     0
		   ) <= $3`,
		organizationID,
		planSlug,
		incomingOrder,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated > 0 {
		return nil
	}

	var exists bool
	if err := r.db.QueryRowContext(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM organizations WHERE id = $1)`,
		organizationID,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("organization %s not found", organizationID)
	}
	return nil
}

func (r *Repository) GetDodoSubscriptionID(ctx context.Context, organizationID string) (string, error) {
	var subscriptionID string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT dodo_subscription_id FROM organizations WHERE id = $1`,
		organizationID,
	).Scan(&subscriptionID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(subscriptionID), nil
}

func (r *Repository) SetDodoSubscriptionID(ctx context.Context, organizationID, subscriptionID string) error {
	organizationID = strings.TrimSpace(organizationID)
	subscriptionID = strings.TrimSpace(subscriptionID)
	if organizationID == "" || subscriptionID == "" {
		return nil
	}
	result, err := r.db.ExecContext(
		ctx,
		`UPDATE organizations SET dodo_subscription_id = $2 WHERE id = $1`,
		organizationID,
		subscriptionID,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return fmt.Errorf("organization %s not found", organizationID)
	}
	return nil
}

func (r *Repository) create(ctx context.Context, name, description, userID, slug string) (*model.Organization, error) {
	org := model.Organization{}
	var planSlug sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO organizations (name, slug, description, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, slug, description, created_by, plan_slug, created_at, updated_at`,
		name,
		slug,
		description,
		userID,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Description, &org.CreatedBy, &planSlug, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, err
	}
	org.PlanSlug = nullString(planSlug)

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
		`SELECT o.id, o.name, o.slug, o.description, o.created_by, o.plan_slug, o.created_at, o.updated_at
		 FROM organizations o
		 INNER JOIN organization_members m ON m.organization_id = o.id
		 WHERE m.user_id = $1`+listForUserOrder,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := make([]model.Organization, 0)
	for rows.Next() {
		org, err := scanListedOrg(rows)
		if err != nil {
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
	var planSlug sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, name, slug, description, created_by, plan_slug,
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
		&planSlug,
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
	org.PlanSlug = nullString(planSlug)
	return &org, nil
}

func (r *Repository) Update(ctx context.Context, id, name, description string) (*model.Organization, error) {
	org := model.Organization{}
	var planSlug sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`UPDATE organizations
		 SET name = $2, description = $3
		 WHERE id = $1
		 RETURNING id, name, slug, description, created_by, plan_slug,
		           credits_granted_micros, credits_used_micros, credits_remaining_micros,
		           created_at, updated_at`,
		id,
		name,
		description,
	).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.CreatedBy,
		&planSlug,
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
	org.PlanSlug = nullString(planSlug)
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
	row := r.db.QueryRowContext(
		ctx,
		`SELECT o.id, o.name, o.slug, o.description, o.created_by, o.plan_slug, o.created_at, o.updated_at
		 FROM organizations o
		 INNER JOIN organization_members m ON m.organization_id = o.id
		 WHERE m.user_id = $1`+listForUserOrder+`
		 LIMIT 1`,
		userID,
	)
	org, err := scanListedOrg(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &org, nil
}

func scanListedOrg(scanner interface{ Scan(dest ...any) error }) (model.Organization, error) {
	var org model.Organization
	var planSlug sql.NullString
	err := scanner.Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.Description,
		&org.CreatedBy,
		&planSlug,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		return model.Organization{}, err
	}
	org.PlanSlug = nullString(planSlug)
	return org, nil
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

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
