package blocklist

import (
	"context"
	"database/sql"
	"time"

	"github.com/shivang-16/orbit.api/internal/logger"
)

func LoadFromDB(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT domain FROM blocked_domains`)
	if err != nil {
		return err
	}
	defer rows.Close()

	domains := make([]string, 0)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return err
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	SetDomains(domains)
	return nil
}

func StartReloader(ctx context.Context, db *sql.DB, every time.Duration) {
	if every < time.Second {
		every = 10 * time.Second
	}
	if err := LoadFromDB(ctx, db); err != nil {
		logger.Error(ctx, "blocklist: initial load failed", "error", err)
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := LoadFromDB(ctx, db); err != nil {
					logger.Error(ctx, "blocklist: reload failed", "error", err)
				}
			}
		}
	}()
}
