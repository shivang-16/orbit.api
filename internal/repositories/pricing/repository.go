package pricing

import (
	"context"
	"database/sql"
	"errors"

	"github.com/shivang-16/orbit.api/internal/model"
)

type dbTX interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Repository struct {
	db dbTX
}

func NewRepository(db dbTX) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByCatalogueID(ctx context.Context, catalogueID string) (*model.ModelPricing, error) {
	item := model.ModelPricing{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, model_catalogue_id,
		        vendor_input_per_million_micros, vendor_output_per_million_micros,
		        currency, created_at, updated_at
		 FROM model_pricing
		 WHERE model_catalogue_id = $1`,
		catalogueID,
	).Scan(
		&item.ID,
		&item.ModelCatalogueID,
		&item.VendorInputPerMillionMicros,
		&item.VendorOutputPerMillionMicros,
		&item.Currency,
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
