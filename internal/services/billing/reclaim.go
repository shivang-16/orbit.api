package billing

import (
	"context"
	"log"
	"time"

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
					log.Printf("billing: reclaim expired holds: %v", err)
					continue
				}
				if n > 0 {
					log.Printf("billing: reclaimed expired holds affecting %d org row(s)", n)
				}
			}
		}
	}()
}
