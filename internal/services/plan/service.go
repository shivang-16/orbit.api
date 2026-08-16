package plan

import (
	"context"
	"fmt"

	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

type Service struct {
	plans *planRepository.Repository
}

func NewService(plans *planRepository.Repository) *Service {
	return &Service{plans: plans}
}

func (s *Service) List(ctx context.Context) (*ListResponse, error) {
	plans, err := s.plans.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	return &ListResponse{Plans: plans, Total: len(plans)}, nil
}
