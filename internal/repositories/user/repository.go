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

func (r *Repository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	user := model.User{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, email, name, image_url, super_admin, created_at, updated_at
		 FROM users
		 WHERE lower(email) = lower($1)`,
		email,
	).Scan(&user.ID, &user.Email, &user.Name, &user.ImageURL, &user.SuperAdmin, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*model.User, error) {
	user := model.User{}
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, email, name, image_url, super_admin, created_at, updated_at
		 FROM users
		 WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.Name, &user.ImageURL, &user.SuperAdmin, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) Create(ctx context.Context, user *model.User) (*model.User, error) {
	created := model.User{}
	err := r.db.QueryRowContext(
		ctx,
		`INSERT INTO users (id, email, name, image_url, super_admin)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, email, name, image_url, super_admin, created_at, updated_at`,
		user.ID, user.Email, user.Name, user.ImageURL, user.SuperAdmin,
	).Scan(&created.ID, &created.Email, &created.Name, &created.ImageURL, &created.SuperAdmin, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &created, nil
}
