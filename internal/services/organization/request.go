package organization

import (
	"strings"
	"unicode/utf8"

	"github.com/shivang-16/orbit.api/internal/model"
)

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r *CreateRequest) isValid() bool {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 80 {
		return false
	}
	if utf8.RuneCountInString(r.Description) > 280 {
		return false
	}
	return true
}

type UpdateRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	OrganizationID string `json:"organization_id"`
}

func (r *UpdateRequest) isValid() bool {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.OrganizationID = strings.TrimSpace(r.OrganizationID)
	if r.Name == "" || utf8.RuneCountInString(r.Name) > 80 {
		return false
	}
	if utf8.RuneCountInString(r.Description) > 280 {
		return false
	}
	return true
}

type AddMemberRequest struct {
	Email          string `json:"email"`
	Role           string `json:"role"`
	OrganizationID string `json:"organization_id"`
}

func (r *AddMemberRequest) isValid() bool {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	r.OrganizationID = strings.TrimSpace(r.OrganizationID)
	r.Role = strings.ToLower(strings.TrimSpace(r.Role))
	if r.Email == "" || !strings.Contains(r.Email, "@") {
		return false
	}
	if r.Role == "" {
		r.Role = string(model.OrgRoleMember)
	}
	switch model.OrgRole(r.Role) {
	case model.OrgRoleAdmin, model.OrgRoleMember:
		return true
	default:
		return false
	}
}
