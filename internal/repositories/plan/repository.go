package plan

import (
	"context"
	"database/sql"
	"errors"

	"github.com/shivang-16/orbit.api/internal/model"
)

type dbTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Repository struct {
	db dbTX
}

func NewRepository(db dbTX) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListActive(ctx context.Context) ([]model.Plan, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, slug, name, dodo_product_id, price_micros, credits_micros,
		        tagline, features, includes_from, highlighted,
		        sort_order, is_active, created_at, updated_at
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
		var item model.Plan
		if err := rows.Scan(
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
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		plans = append(plans, item)
	}
	return plans, rows.Err()
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*model.Plan, error) {
	return r.getOne(ctx, `WHERE slug = $1 AND is_active = true`, slug)
}

func (r *Repository) GetByDodoProductID(ctx context.Context, dodoProductID string) (*model.Plan, error) {
	return r.getOne(ctx, `WHERE dodo_product_id = $1`, dodoProductID)
}

func (r *Repository) getOne(ctx context.Context, where string, arg string) (*model.Plan, error) {
	var item model.Plan
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, slug, name, dodo_product_id, price_micros, credits_micros,
		        tagline, features, includes_from, highlighted,
		        sort_order, is_active, created_at, updated_at
		 FROM plans `+where,
		arg,
	).Scan(
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
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
