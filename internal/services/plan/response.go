package plan

import "github.com/shivang-16/orbit.api/internal/model"

type ListResponse struct {
	Plans []model.Plan `json:"plans"`
	Total int          `json:"total"`
}
