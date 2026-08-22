package catalogue

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shivang-16/orbit.api/internal/model"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
)

// Order of the highlight cards on the overview page.
var highlightTags = []string{"flagship", "open-source", "cost-efficient", "fast", "coding"}

type Service struct {
	catalogue *catalogueRepository.Repository
	pricing   *pricingRepository.Repository
}

func NewService(catalogue *catalogueRepository.Repository, pricing *pricingRepository.Repository) *Service {
	return &Service{catalogue: catalogue, pricing: pricing}
}

func (s *Service) List(ctx context.Context, tag string, sortBy string) (*ListResponse, error) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	sortBy = strings.TrimSpace(strings.ToLower(sortBy))
	models, err := s.catalogue.ListActive(ctx, tag, sortBy)
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

	price, err := s.pricing.GetByCatalogueID(ctx, item.ID)
	if err != nil {
		return nil, fmt.Errorf("get model pricing: %w", err)
	}

	return &GetResponse{Model: *item, Pricing: price}, nil
}

func (s *Service) Overview(ctx context.Context) (*OverviewResponse, error) {
	models, err := s.catalogue.ListActive(ctx, "", "")
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
		Frontier:     limit(newestFirst(rankedByTag(models, "flagship")), 4),
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

// newestFirst orders models by real release date, newest first, keeping
// the input order (vendor, then rankedByTag's sort_order/name tie-break)
// for models without a recorded date so the "frontier" ranking is
// genuinely newest-first rather than an editorial guess.
func newestFirst(models []model.ModelCatalogue) []model.ModelCatalogue {
	out := make([]model.ModelCatalogue, len(models))
	copy(out, models)
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].ModelReleasedDate, out[j].ModelReleasedDate
		switch {
		case di == nil && dj == nil:
			return false
		case di == nil:
			return false // undated models sort after dated ones
		case dj == nil:
			return true
		default:
			return di.After(*dj)
		}
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
