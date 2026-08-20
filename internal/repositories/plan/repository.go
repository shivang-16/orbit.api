package plan

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

	"github.com/shivang-16/orbit.api/internal/model"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type dbTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const planColumns = `id, slug, name, dodo_product_id, price_micros, credits_micros,
		tagline, features, includes_from, highlighted,
		sort_order, is_active, max_organizations, max_members_per_org,
		created_at, updated_at`

type Repository struct {
	db dbTX
}

func NewRepository(db dbTX) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListActive(ctx context.Context) ([]model.Plan, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT `+planColumns+`
		 FROM plans
		 WHERE is_active = true
		 ORDER BY sort_order ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := make([]model.Plan, 0)
	for rows.Next() {
		item, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, item)
	}
	return plans, rows.Err()
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*model.Plan, error) {
	return r.getOne(ctx, `WHERE slug = $1 AND is_active = true`, slug)
}

// GetByID looks up a plan by its UUID primary key. Checkout metadata stores
// this alongside plan_slug so the webhook can resolve the plan even if a
// provider strips or renames the slug field. Malformed input returns
// (nil, nil) rather than a Postgres uuid-cast error, since webhook metadata
// is untrusted echo-back data.
func (r *Repository) GetByID(ctx context.Context, id string) (*model.Plan, error) {
	if !uuidPattern.MatchString(id) {
		return nil, nil
	}
	return r.getOne(ctx, `WHERE id = $1`, id)
}

func (r *Repository) GetByDodoProductID(ctx context.Context, dodoProductID string) (*model.Plan, error) {
	return r.getOne(ctx, `WHERE dodo_product_id = $1`, dodoProductID)
}

func (r *Repository) getOne(ctx context.Context, where string, arg string) (*model.Plan, error) {
	item, err := scanPlan(r.db.QueryRowContext(
		ctx,
		`SELECT `+planColumns+` FROM plans `+where,
		arg,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanPlan(scanner interface{ Scan(dest ...any) error }) (model.Plan, error) {
	var item model.Plan
	var maxOrgs, maxMembers sql.NullInt64
	err := scanner.Scan(
		&item.ID,
		&item.Slug,
		&item.Name,
		&item.DodoProductID,
		&item.PriceMicros,
		&item.CreditsMicros,
		&item.Tagline,
		&item.Features,
		&item.IncludesFrom,
		&item.Highlighted,
		&item.SortOrder,
		&item.IsActive,
		&maxOrgs,
		&maxMembers,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.Plan{}, err
	}
	item.MaxOrganizations = nullIntPtr(maxOrgs)
	item.MaxMembersPerOrg = nullIntPtr(maxMembers)
	return item, nil
}

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}
