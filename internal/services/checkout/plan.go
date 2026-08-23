package checkout

import "github.com/shivang-16/orbit.api/internal/model"

func planChangeError(current, target *model.Plan) error {
	if current == nil || target == nil {
		return nil
	}
	if current.Slug == target.Slug {
		return ErrSamePlan
	}
	if target.SortOrder < current.SortOrder {
		return ErrDowngrade
	}
	return nil
}
