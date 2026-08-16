package organization

import "github.com/shivang-16/orbit.api/internal/model"

type ListResponse struct {
	Organizations []model.Organization `json:"organizations"`
	Total         int                  `json:"total"`
}

type CreateResponse struct {
	Organization model.Organization `json:"organization"`
}
