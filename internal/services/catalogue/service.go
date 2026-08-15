package catalogue

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shivang-16/orbit.api/internal/model"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
)

// Order of the highlight cards on the overview page.
var highlightTags = []string{"flagship", "open-source", "cost-efficient", "fast", "coding"}

type Service struct {
	catalogue *catalogueRepository.Repository
}

func NewService(catalogue *catalogueRepository.Repository) *Service {
	return &Service{catalogue: catalogue}
}

func (s *Service) List(ctx context.Context, tag string) (*ListResponse, error) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	models, err := s.catalogue.ListActive(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("list catalogue: %w", err)
	}
	return &ListResponse{Models: models, Total: len(models)}, nil
}

func (s *Service) Get(ctx context.Context, id string) (*GetResponse, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	item, err := s.catalogue.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get catalogue model: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return &GetResponse{Model: *item}, nil
}

func (s *Service) Overview(ctx context.Context) (*OverviewResponse, error) {
	models, err := s.catalogue.ListActive(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("list catalogue: %w", err)
	}

	highlights := make([]Highlight, 0, len(highlightTags))
	for _, tag := range highlightTags {
		ranked := rankedByTag(models, tag)
		highlights = append(highlights, Highlight{
			Tag:   tag,
			Model: first(ranked),
			Count: len(ranked),
		})
	}

	return &OverviewResponse{
		Total:        len(models),
		Frontier:     limit(rankedByTag(models, "flagship"), 4),
		Highlights:   highlights,
		ValueLeaders: limit(rankedByTag(models, "cost-efficient"), 5),
		Fastest:      limit(rankedByTag(models, "fast"), 5),
		Tags:         tagSummaries(models),
	}, nil
}

func rankedByTag(models []model.ModelCatalogue, tag string) []model.ModelCatalogue {
	out := make([]model.ModelCatalogue, 0)
	for _, item := range models {
		if hasTag(item, tag) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func tagSummaries(models []model.ModelCatalogue) []TagSummary {
	counts := map[string]int{}
	for _, item := range models {
		for _, tag := range item.Tags {
			counts[tag]++
		}
	}

	out := make([]TagSummary, 0, len(counts))
	for tag, count := range counts {
		out = append(out, TagSummary{Tag: tag, Count: count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

func hasTag(item model.ModelCatalogue, tag string) bool {
	for _, value := range item.Tags {
		if value == tag {
			return true
		}
	}
	return false
}

func first(models []model.ModelCatalogue) *model.ModelCatalogue {
	if len(models) == 0 {
		return nil
	}
	item := models[0]
	return &item
}

func limit(models []model.ModelCatalogue, n int) []model.ModelCatalogue {
	if len(models) <= n {
		return models
	}
	return models[:n]
}
