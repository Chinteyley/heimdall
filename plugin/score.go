package heimdall

import (
	"math"
	"time"
)

// horizonDays is how long evidence about each category stays good: past it,
// a confirming source no longer proves the fact is still true today.
var horizonDays = map[Category]float64{
	CategoryPricing:    90,
	CategoryVersion:    60,
	CategoryLeadership: 180,
	CategoryLaw:        180,
	CategoryCompany:    120,
	CategoryStatistic:  180,
	CategoryOther:      180,
}

// Score is a claim's freshness, 0 to 100: how sure we are it still holds today.
//
//	current:      60 + 25·recency + 15·agreement   (60–100)
//	stale:        15·(1 − recency)                  (0–15; fresh contradicting evidence scores 0)
//	unverifiable: 40 for pricing and versions, 50 otherwise
//
// recency is 1 for evidence dated today, falling linearly to 0 at the
// category's horizon, and 0.5 when no source is dated. agreement is the number
// of sources the verdict rests on, capped at three, over three.
func Score(status Status, cat Category, evidenceDate string, sources int, now time.Time) int {
	switch status {
	case StatusCurrent:
		return round(60 + 25*recency(cat, evidenceDate, now) + 15*float64(min(sources, 3))/3)
	case StatusStale:
		return round(15 * (1 - recency(cat, evidenceDate, now)))
	default:
		if cat == CategoryPricing || cat == CategoryVersion {
			return 40
		}
		return 50
	}
}

func recency(cat Category, date string, now time.Time) float64 {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0.5
	}
	horizon, ok := horizonDays[cat]
	if !ok {
		horizon = horizonDays[CategoryOther]
	}
	age := now.Sub(t).Hours() / 24
	return math.Max(0, math.Min(1, 1-age/horizon))
}

func round(f float64) int { return int(math.Round(f)) }
