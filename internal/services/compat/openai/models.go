package openai

import (
	"encoding/json"

	"github.com/shivang-16/orbit.api/internal/model"
)

type modelListResponse struct {
	Object string           `json:"object"`
	Data   []modelListEntry `json:"data"`
}

type modelListEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// NewModelListResponse formats the active catalogue as an OpenAI-style
// GET /v1/models response, keyed by each model's public slug so it can be
// passed straight back as the "model" field of a chat.completions call.
func NewModelListResponse(models []model.ModelCatalogue) []byte {
	data := make([]modelListEntry, len(models))
	for i, m := range models {
		data[i] = modelListEntry{
			ID:      m.Slug,
			Object:  "model",
			Created: m.CreatedAt.Unix(),
			OwnedBy: m.Vendor,
		}
	}
	resp := modelListResponse{Object: "list", Data: data}
	body, _ := json.Marshal(resp)
	return body
}
