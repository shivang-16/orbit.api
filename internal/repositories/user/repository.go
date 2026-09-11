package user

import (
	"context"
	"database/sql"
	"errors"

	"github.com/shivang-16/orbit.api/internal/model"
)

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

const userColumns = `id, email, name, image_url, super_admin, blocked, created_at, updated_at`

func scanUser(row *sql.Row) (*model.User, error) {
	user := model.User{}
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.ImageURL,
		&user.SuperAdmin,
		&user.Blocked,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return scanUser(r.db.QueryRowContext(
		ctx,
		`SELECT `+userColumns+`
		 FROM users
		 WHERE lower(email) = lower($1)`,
		email,
	))
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.User, error) {
	return scanUser(r.db.QueryRowContext(
		ctx,
		`SELECT `+userColumns+`
		 FROM users
		 WHERE id = $1`,
		id,
	))
}

func (r *Repository) GetOwnerByOrganizationID(ctx context.Context, organizationID string) (*model.User, error) {
	return scanUser(r.db.QueryRowContext(
		ctx,
		`SELECT `+userColumns+`
		 FROM users u
		 JOIN organizations o ON o.created_by = u.id
		 WHERE o.id = $1`,
		organizationID,
	))
}

func (r *Repository) Create(ctx context.Context, user *model.User) (*model.User, error) {
	return scanUser(r.db.QueryRowContext(
		ctx,
		`INSERT INTO users (id, email, name, image_url, super_admin, blocked)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+userColumns,
		user.ID, user.Email, user.Name, user.ImageURL, user.SuperAdmin, user.Blocked,
	))
}

func (r *Repository) SetBlocked(ctx context.Context, id string, blocked bool) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE users SET blocked = $2 WHERE id = $1`,
		id,
		blocked,
	)
	return err
}
