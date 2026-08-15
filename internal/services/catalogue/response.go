package catalogue

import "github.com/shivang-16/orbit.api/internal/model"

type ListResponse struct {
	Models []model.ModelCatalogue `json:"models"`
	Total  int                    `json:"total"`
}

type GetResponse struct {
	Model model.ModelCatalogue `json:"model"`
}

// Highlight is the leading model for a tag, plus how many models carry it.
type Highlight struct {
	Tag   string                `json:"tag"`
	Model *model.ModelCatalogue `json:"model"`
	Count int                   `json:"count"`
}

type TagSummary struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type OverviewResponse struct {
	Total        int                    `json:"total"`
	Frontier     []model.ModelCatalogue `json:"frontier"`
	Highlights   []Highlight            `json:"highlights"`
	ValueLeaders []model.ModelCatalogue `json:"value_leaders"`
	Fastest      []model.ModelCatalogue `json:"fastest"`
	Tags         []TagSummary           `json:"tags"`
}
