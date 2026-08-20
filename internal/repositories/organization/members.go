package organization

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	"github.com/shivang-16/orbit.api/internal/model"
)

var ErrAlreadyMember = errors.New("already a member")

type Member struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"organization_id"`
	UserID         string        `json:"user_id"`
	Email          string        `json:"email"`
	Name           string        `json:"name"`
	ImageURL       string        `json:"image_url"`
	Role           model.OrgRole `json:"role"`
	CreatedAt      time.Time     `json:"created_at"`
}

func (r *Repository) GetRole(ctx context.Context, userID, organizationID string) (model.OrgRole, bool, error) {
	var role model.OrgRole
	err := r.db.QueryRowContext(
		ctx,
		`SELECT role FROM organization_members
		 WHERE user_id = $1 AND organization_id = $2`,
		userID,
		organizationID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return role, true, nil
}

func (r *Repository) CountMembers(ctx context.Context, organizationID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM organization_members WHERE organization_id = $1`,
		organizationID,
	).Scan(&count)
	return count, err
}

func (r *Repository) ListMembers(ctx context.Context, organizationID string) ([]Member, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT m.id, m.organization_id, m.user_id, u.email, u.name, u.image_url, m.role, m.created_at
		 FROM organization_members m
		 INNER JOIN users u ON u.id = m.user_id
		 WHERE m.organization_id = $1
		 ORDER BY m.created_at ASC`,
		organizationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]Member, 0)
	for rows.Next() {
		var item Member
		if err := rows.Scan(
			&item.ID,
			&item.OrganizationID,
			&item.UserID,
			&item.Email,
			&item.Name,
			&item.ImageURL,
			&item.Role,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		members = append(members, item)
	}
	return members, rows.Err()
}

func (r *Repository) AddMember(ctx context.Context, organizationID, userID string, role model.OrgRole) (*Member, error) {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO organization_members (organization_id, user_id, role)
		 VALUES ($1, $2, $3)`,
		organizationID,
		userID,
		role,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, ErrAlreadyMember
		}
		return nil, err
	}

	members, err := r.ListMembers(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].UserID == userID {
			return &members[i], nil
		}
	}
	return nil, sql.ErrNoRows
}
