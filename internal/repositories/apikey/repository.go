package apikey

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shivang-16/orbit.api/internal/model"
)

const apiKeyColumns = `id, organization_id, created_by, name, key_preview, status,
		        expires_at, last_used_at, created_at, updated_at`

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

	item, err := scanAPIKey(r.db.QueryRowContext(
		ctx,
		`INSERT INTO api_keys (organization_id, created_by, name, key_hash, key_preview, status, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+apiKeyColumns,
		params.OrganizationID,
		params.CreatedBy,
		params.Name,
		params.KeyHash,
		params.KeyPreview,
		model.APIKeyStatusActive,
		expiresAt,
	).Scan)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *Repository) ListByOrganization(ctx context.Context, organizationID string) ([]model.APIKey, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT `+apiKeyColumns+`
		 FROM api_keys
		 WHERE organization_id = $1
		   AND status = $2
		   AND revoked_at IS NULL
		 ORDER BY created_at DESC`,
		organizationID,
		model.APIKeyStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]model.APIKey, 0)
	for rows.Next() {
		item, err := scanAPIKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

// GetActiveByHash resolves a usable key by its hash in a single indexed
// lookup, filtering out inactive, revoked, or expired keys in the same
// query so the hot inference path never needs a second round trip.
func (r *Repository) GetActiveByHash(ctx context.Context, hash string) (*model.APIKey, error) {
	item, err := scanAPIKey(r.db.QueryRowContext(
		ctx,
		`SELECT `+apiKeyColumns+`
		 FROM api_keys
		 WHERE key_hash = $1
		   AND status = $2
		   AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > now())`,
		hash,
		model.APIKeyStatusActive,
	).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

// Deactivate marks an active key inactive for the given organization.
// Returns false when no matching active key exists.
func (r *Repository) Deactivate(ctx context.Context, id, organizationID string) (bool, error) {
	res, err := r.db.ExecContext(
		ctx,
		`UPDATE api_keys
		 SET status = $1, revoked_at = now()
		 WHERE id = $2
		   AND organization_id = $3
		   AND status = $4`,
		model.APIKeyStatusInactive,
		id,
		organizationID,
		model.APIKeyStatusActive,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// TouchLastUsed records key usage. Callers should fire this off in a
// goroutine so it never adds latency to the request the key authenticated.
func (r *Repository) TouchLastUsed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	return err
}

func scanAPIKey(scan func(dest ...any) error) (*model.APIKey, error) {
	item := model.APIKey{}
	var expiresScan, lastUsedScan sql.NullTime
	if err := scan(
		&item.ID,
		&item.OrganizationID,
		&item.CreatedBy,
		&item.Name,
		&item.KeyPreview,
		&item.Status,
		&expiresScan,
		&lastUsedScan,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	item.ExpiresAt = nullTime(expiresScan)
	item.LastUsedAt = nullTime(lastUsedScan)
	return &item, nil
}

func nullTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
