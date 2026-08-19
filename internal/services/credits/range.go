package credits

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidRange = fmt.Errorf("invalid range")
	ErrInvalidPage  = fmt.Errorf("invalid page")
)

const (
	defaultUsagePage  = 1
	defaultUsageLimit = 25
)

var allowedUsageLimits = map[int]struct{}{
	25: {},
	50: {},
	75: {},
}

// UsageRange is a half-open [From, To) window in UTC. To is exclusive
// and always the first instant after the last included calendar day.
type UsageRange struct {
	Preset string
	From   time.Time
	To     time.Time
}

// parseUsageRange maps a UI preset onto calendar days in UTC.
//
//	1d          today
//	7d          today and the previous 6 days (default)
//	30d         today and the previous 29 days
//	mtd         first of this month through today
//	last_month  the previous calendar month
func parseUsageRange(preset string, now time.Time) (UsageRange, error) {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	tomorrow := today.AddDate(0, 0, 1)

	preset = strings.TrimSpace(strings.ToLower(preset))
	if preset == "" {
		preset = "7d"
	}

	switch preset {
	case "1d":
		return UsageRange{Preset: preset, From: today, To: tomorrow}, nil
	case "7d":
		return UsageRange{Preset: preset, From: today.AddDate(0, 0, -6), To: tomorrow}, nil
	case "30d":
		return UsageRange{Preset: preset, From: today.AddDate(0, 0, -29), To: tomorrow}, nil
	case "mtd":
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return UsageRange{Preset: preset, From: from, To: tomorrow}, nil
	case "last_month":
		firstThis := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return UsageRange{Preset: preset, From: firstThis.AddDate(0, -1, 0), To: firstThis}, nil
	default:
		return UsageRange{}, ErrInvalidRange
	}
}

func parseUsagePage(pageRaw, limitRaw string) (page, limit int, err error) {
	limit = defaultUsageLimit
	if strings.TrimSpace(limitRaw) != "" {
		n, convErr := strconv.Atoi(strings.TrimSpace(limitRaw))
		if convErr != nil {
			return 0, 0, ErrInvalidPage
		}
		if _, ok := allowedUsageLimits[n]; !ok {
			return 0, 0, ErrInvalidPage
		}
		limit = n
	}

	page = defaultUsagePage
	if strings.TrimSpace(pageRaw) != "" {
		n, convErr := strconv.Atoi(strings.TrimSpace(pageRaw))
		if convErr != nil || n < 1 {
			return 0, 0, ErrInvalidPage
		}
		page = n
	}
	return page, limit, nil
}

func eachUTCDay(from, to time.Time) []time.Time {
	from = time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.UTC().Year(), to.UTC().Month(), to.UTC().Day(), 0, 0, 0, 0, time.UTC)
	days := make([]time.Time, 0)
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		days = append(days, day)
	}
	return days
}
