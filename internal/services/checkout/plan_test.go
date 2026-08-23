package checkout

import (
	"errors"
	"testing"

	"github.com/shivang-16/orbit.api/internal/model"
)

func TestPlanChangeError(t *testing.T) {
	trial := &model.Plan{Slug: "trial", SortOrder: 1}
	starter := &model.Plan{Slug: "starter", SortOrder: 2}

	if err := planChangeError(nil, starter); err != nil {
		t.Fatalf("no current plan: %v", err)
	}
	if err := planChangeError(trial, trial); !errors.Is(err, ErrSamePlan) {
		t.Fatalf("same plan: %v", err)
	}
	if err := planChangeError(starter, trial); !errors.Is(err, ErrDowngrade) {
		t.Fatalf("downgrade: %v", err)
	}
	if err := planChangeError(trial, starter); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
}
