package billing

import (
	"context"
	"time"

	"github.com/shivang-16/orbit.api/internal/logger"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
)

func StartHoldReclaimer(ctx context.Context, billing *billingRepository.Repository) {
	if billing == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reclaimCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				n, err := billing.ReleaseExpiredHolds(reclaimCtx)
				cancel()
				if err != nil {
					logger.Error(logger.SetTag(reclaimCtx, logger.TagBilling), "billing: reclaim expired holds failed", "error", err)
					continue
				}
				if n > 0 {
					logger.Info(logger.SetTag(reclaimCtx, logger.TagBilling), "billing: reclaimed expired holds", "org_rows", n)
				}
			}
		}
	}()
}
