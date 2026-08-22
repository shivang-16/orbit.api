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

const catalogueColumns = `id, name, slug, vendor, provider, model_id, input_context_limit,
		       sort_order, tags, modalities, is_active, model_released_date, created_at, updated_at`

// ListActive returns active catalogue models, optionally filtered by tag.
// sort controls ordering: "newest"/"oldest" order by model_released_date
// (nulls last), anything else (including "") keeps the default
// vendor/sort_order ordering.
func (r *Repository) ListActive(ctx context.Context, tag string, sort string) ([]model.ModelCatalogue, error) {
	query := `
		SELECT ` + catalogueColumns + `
		FROM model_catalogue
		WHERE is_active = true
	`
	args := []any{}
	if tag != "" {
		query += ` AND tags @> $1`
		args = append(args, pq.Array([]string{tag}))
	}
	switch sort {
	case "newest":
		query += ` ORDER BY model_released_date DESC NULLS LAST, vendor ASC, sort_order ASC`
	case "oldest":
		query += ` ORDER BY model_released_date ASC NULLS LAST, vendor ASC, sort_order ASC`
	default:
		query += ` ORDER BY vendor ASC, sort_order ASC`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]model.ModelCatalogue, 0)
	for rows.Next() {
		item, err := scanCatalogue(rows.Scan)
		if err != nil {
			return nil, err
		}
		models = append(models, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return models, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.ModelCatalogue, error) {
	item, err := scanCatalogue(r.db.QueryRowContext(
		ctx,
		`SELECT `+catalogueColumns+`
		 FROM model_catalogue
		 WHERE id = $1 AND is_active = true`,
		id,
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

// GetByIdentifier resolves a model by either its public slug (the identifier
// OpenAI/Anthropic-compatible callers pass as "model") or its catalogue
// UUID (used by the native /models/{id}/chat route). Comparing id as text
// avoids a Postgres error when identifier isn't a valid UUID.
func (r *Repository) GetByIdentifier(ctx context.Context, identifier string) (*model.ModelCatalogue, error) {
	item, err := scanCatalogue(r.db.QueryRowContext(
		ctx,
		`SELECT `+catalogueColumns+`
		 FROM model_catalogue
		 WHERE is_active = true AND (slug = $1 OR id::text = $1)`,
		identifier,
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func scanCatalogue(scan func(dest ...any) error) (*model.ModelCatalogue, error) {
	item := model.ModelCatalogue{}
	if err := scan(
		&item.ID,
		&item.Name,
		&item.Slug,
		&item.Vendor,
		&item.Provider,
		&item.ModelID,
		&item.InputContextLimit,
		&item.SortOrder,
		&item.Tags,
		&item.Modalities,
		&item.IsActive,
		&item.ModelReleasedDate,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}
