package heimdall

import "time"

// Mode is what Heimdall does to a response, chosen per request with the
// x-heimdall header or by Config.DefaultMode.
type Mode string

const (
	ModeOff      Mode = "off"      // pass through untouched
	ModeReport   Mode = "report"   // answer untouched, report attached
	ModeAnnotate Mode = "annotate" // stale values struck through inline, sources as footnotes
	ModeCorrect  Mode = "correct"  // answer rewritten from the evidence
	ModeCheck    Mode = "check"    // no model call: the last user message is the answer to check
)

func parseMode(s string) (Mode, bool) {
	switch m := Mode(s); m {
	case ModeOff, ModeReport, ModeAnnotate, ModeCorrect, ModeCheck:
		return m, true
	}
	return "", false
}

// Category is the kind of fact a claim states; it sets how fast the fact decays.
type Category string

const (
	CategoryPricing    Category = "pricing"
	CategoryVersion    Category = "version"
	CategoryLeadership Category = "leadership"
	CategoryLaw        Category = "law"
	CategoryCompany    Category = "company"
	CategoryStatistic  Category = "statistic"
	CategoryOther      Category = "other"
)

// Status is the verdict on one claim after the live check.
type Status string

const (
	StatusCurrent      Status = "current"
	StatusStale        Status = "stale"
	StatusUnverifiable Status = "unverifiable"
)

// Claim is one time-sensitive statement found in an answer.
type Claim struct {
	ID       int      `json:"id"`
	Text     string   `json:"text"`     // the claim restated on its own
	Quote    string   `json:"quote"`    // verbatim span of the answer
	Value    string   `json:"value"`    // the part that can go stale, inside Quote
	Category Category `json:"category"` // sets how fast the fact decays
	Query    string   `json:"query"`    // web search query that would settle it
	Domain   string   `json:"domain"`   // the entity's official site, searched first; may be empty
}

// Source is one web page the verdict rests on.
type Source struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	PublishedDate string `json:"published_date,omitempty"`
}

// CheckedClaim is a claim with its verdict and freshness score.
type CheckedClaim struct {
	Claim
	Status       Status   `json:"status"`
	Freshness    int      `json:"freshness"` // 0 (stale) to 100 (confirmed fresh)
	CurrentValue string   `json:"current_value,omitempty"`
	Explanation  string   `json:"explanation"`
	EvidenceDate string   `json:"evidence_date,omitempty"` // newest dated source used, YYYY-MM-DD
	Sources      []Source `json:"sources"`
}

// Report is what Heimdall attaches to a response under the "heimdall" key.
type Report struct {
	Mode      Mode           `json:"mode"`
	CheckedAt time.Time      `json:"checked_at"`
	Score     int            `json:"score"` // lowest claim freshness; 100 when nothing time-sensitive
	Stale     int            `json:"stale"`
	Claims    []CheckedClaim `json:"claims"`
	Usage     Usage          `json:"usage"`
	Error     string         `json:"error,omitempty"`
}

// Usage counts what one check spent.
type Usage struct {
	Searches   int   `json:"tavily_searches"`
	Credits    int   `json:"tavily_credits"`
	ModelCalls int   `json:"model_calls"`
	LatencyMs  int64 `json:"latency_ms"`
}
