package apikey

import "github.com/shivang-16/orbit.api/internal/model"

type ListResponse struct {
	Keys      []model.APIKey `json:"keys"`
	Total     int            `json:"total"`
	Role      model.OrgRole  `json:"role"`
	CanDelete bool           `json:"can_delete"`
}

type CreateResponse struct {
	Key    model.APIKey `json:"key"`
	Secret string       `json:"secret"`
}
