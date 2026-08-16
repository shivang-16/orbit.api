package apikey

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shivang-16/orbit.api/internal/model"
)

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

type CreateParams struct {
	OrganizationID string
	CreatedBy      string
	Name           string
	KeyHash        string
	KeyPreview     string
	ExpiresAt      *time.Time
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (*model.APIKey, error) {
	var expiresAt sql.NullTime
	if params.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *params.ExpiresAt, Valid: true}
	}

	item := model.APIKey{}
	var expiresScan, lastUsedScan sql.NullTime
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO api_keys (organization_id, created_by, name, key_hash, key_preview, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, organization_id, created_by, name, key_preview,
		           expires_at, last_used_at, created_at, updated_at`,
		params.OrganizationID,
		params.CreatedBy,
		params.Name,
		params.KeyHash,
		params.KeyPreview,
		expiresAt,
	).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.CreatedBy,
		&item.Name,
		&item.KeyPreview,
		&expiresScan,
		&lastUsedScan,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.ExpiresAt = nullTime(expiresScan)
	item.LastUsedAt = nullTime(lastUsedScan)
	return &item, nil
}

func (r *Repository) ListByOrganization(ctx context.Context, organizationID string) ([]model.APIKey, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, organization_id, created_by, name, key_preview,
		        expires_at, last_used_at, created_at, updated_at
		 FROM api_keys
		 WHERE organization_id = $1 AND revoked_at IS NULL
		 ORDER BY created_at DESC`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]model.APIKey, 0)
	for rows.Next() {
		var item model.APIKey
		var expiresScan, lastUsedScan sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.OrganizationID,
			&item.CreatedBy,
			&item.Name,
			&item.KeyPreview,
			&expiresScan,
			&lastUsedScan,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.ExpiresAt = nullTime(expiresScan)
		item.LastUsedAt = nullTime(lastUsedScan)
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

// GetActiveByHash resolves a usable key by its hash in a single indexed
// lookup, filtering out revoked or expired keys in the same query so the
// hot inference path never needs a second round trip.
func (r *Repository) GetActiveByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	item := model.APIKey{}
	var expiresScan, lastUsedScan sql.NullTime
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, organization_id, created_by, name, key_preview,
		        expires_at, last_used_at, created_at, updated_at
		 FROM api_keys
		 WHERE key_hash = $1
		   AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())`,
		hash,
	).Scan(
		&item.ID,
		&item.OrganizationID,
		&item.CreatedBy,
		&item.Name,
		&item.KeyPreview,
		&expiresScan,
		&lastUsedScan,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.ExpiresAt = nullTime(expiresScan)
	item.LastUsedAt = nullTime(lastUsedScan)
	return &item, nil
}

// TouchLastUsed records key usage. Callers should fire this off in a
// goroutine so it never adds latency to the request the key authenticated.
func (r *Repository) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	return err
}

func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
