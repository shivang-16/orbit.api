package catalogue

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"

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

func (r *Repository) ListActive(ctx context.Context, tag string) ([]model.ModelCatalogue, error) {
	query := `
		SELECT id, name, vendor, provider, model_id, input_context_limit,
		       sort_order, tags, modalities, is_active, created_at, updated_at
		FROM model_catalogue
		WHERE is_active = true
	`
	args := []any{}
	if tag != "" {
		query += ` AND tags @> $1`
		args = append(args, pq.Array([]string{tag}))
	}
	query += ` ORDER BY vendor ASC, sort_order ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]model.ModelCatalogue, 0)
	for rows.Next() {
		var item model.ModelCatalogue
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Vendor,
			&item.Provider,
			&item.ModelID,
			&item.InputContextLimit,
			&item.SortOrder,
			&item.Tags,
			&item.Modalities,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		models = append(models, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.ModelCatalogue, error) {
	var item model.ModelCatalogue
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, name, vendor, provider, model_id, input_context_limit,
		        sort_order, tags, modalities, is_active, created_at, updated_at
		 FROM model_catalogue
		 WHERE id = $1 AND is_active = true`,
		id,
	).Scan(
		&item.ID,
		&item.Name,
		&item.Vendor,
		&item.Provider,
		&item.ModelID,
		&item.InputContextLimit,
		&item.SortOrder,
		&item.Tags,
		&item.Modalities,
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
