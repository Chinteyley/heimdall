package heimdall

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const extractSystem = `You find claims in an AI assistant's answer that may have gone out of date since the model was trained.
Today is %s.
Return only facts that change over time and can be checked on the web: prices and plans, model, API, library or product versions and their availability, who holds a role (CEO, minister, maintainer), laws, regulations and their dates, company facts (headcount, funding, acquisitions, offices), and fast-moving statistics.
Skip opinions, advice, definitions, history that cannot change, and code.
For each claim:
- text: the claim restated so it stands alone (name the entity).
- quote: copy the shortest span of the answer that contains the claim, character for character.
- value: the part of quote that would change if the claim went stale (a price, a version, a name); must appear inside quote exactly.
- category: pricing, version, leadership, law, company, statistic or other.
- query: a web search query whose results settle whether the claim is still true today. Ask what is true now (e.g. "latest Next.js version"), not whether the old value is right, and do not repeat the old value.
- domain: the official website that would publish this fact (e.g. openai.com for OpenAI prices, nextjs.org for Next.js releases), or "" if there is none.
Return at most %d claims, most consequential first. Return an empty list when there are none.`

const judgeSystem = `You check one claim from an AI assistant's answer against live web search results.
Today is %s. The search results are numbered; their published dates are given when known.
Decide:
- current: the newest reliable evidence still supports the claim.
- stale: newer reliable evidence shows the value is now wrong. Give current_value: only the replacement for the claim's value, as short as the value itself (e.g. "$2.50", "16.4", "Lip-Bu Tan"), in the same units and form. If there is no single replacement, leave current_value "" and say why in explanation.
A claim that is still true is current even if it is worded differently, less precise, or describes the same date another way; stale means the stated value no longer holds.
- unverifiable: the evidence does not settle it either way.
Read the claim as the answer meant it: "latest", "current" or "newest" means newest overall today, not the newest in the same major version line, and current_value must then be that newest overall value.
Results marked (official source) come from the entity's own site; prefer them, and the newest dates, over third-party copies (blogs, aggregators) when they disagree. Do not use your own memory for facts; only the results.
explanation: one short sentence, citing what changed and when if stale.
source_ids: the result numbers the verdict rests on, best first.
evidence_date: the published date (YYYY-MM-DD) of the newest result you relied on, or "" when none is dated.`

var extractSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []string{"claims"},
	"properties": map[string]any{
		"claims": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"text", "quote", "value", "category", "query", "domain"},
				"properties": map[string]any{
					"text":  map[string]any{"type": "string"},
					"quote": map[string]any{"type": "string"},
					"value": map[string]any{"type": "string"},
					"category": map[string]any{"type": "string", "enum": []string{
						string(CategoryPricing), string(CategoryVersion), string(CategoryLeadership), string(CategoryLaw),
						string(CategoryCompany), string(CategoryStatistic), string(CategoryOther)}},
					"query":  map[string]any{"type": "string"},
					"domain": map[string]any{"type": "string"},
				},
			},
		},
	},
}

var judgeSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"status", "current_value", "explanation", "source_ids", "evidence_date"},
	"properties": map[string]any{
		"status":        map[string]any{"type": "string", "enum": []string{string(StatusCurrent), string(StatusStale), string(StatusUnverifiable)}},
		"current_value": map[string]any{"type": "string"},
		"explanation":   map[string]any{"type": "string"},
		"source_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"evidence_date": map[string]any{"type": "string"},
	},
}

// Check runs the whole pipeline on one answer: extract claims, search each,
// judge each against its evidence, and score it.
func (p *Plugin) Check(ctx context.Context, answer string, mode Mode) Report {
	start := p.now()
	rep := Report{Mode: mode, CheckedAt: start.UTC(), Score: 100, Claims: []CheckedClaim{}}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	defer cancel()

	claims, err := p.extract(ctx, answer)
	rep.Usage.ModelCalls++
	if err != nil {
		rep.Error = "extract: " + err.Error()
		rep.Usage.LatencyMs = p.now().Sub(start).Milliseconds()
		return rep
	}

	checked := make([]CheckedClaim, len(claims))
	usage := make([]Usage, len(claims))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, c := range claims {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			checked[i], usage[i] = p.verify(ctx, c, start)
		}()
	}
	wg.Wait()

	for i, c := range checked {
		rep.Usage.Searches += usage[i].Searches
		rep.Usage.Credits += usage[i].Credits
		rep.Usage.ModelCalls += usage[i].ModelCalls
		if c.Status == StatusStale {
			rep.Stale++
		}
		rep.Score = min(rep.Score, c.Freshness)
	}
	rep.Claims = checked
	rep.Usage.LatencyMs = p.now().Sub(start).Milliseconds()
	return rep
}

func (p *Plugin) extract(ctx context.Context, answer string) ([]Claim, error) {
	var out struct {
		Claims []Claim `json:"claims"`
	}
	system := fmt.Sprintf(extractSystem, p.today(), p.cfg.MaxClaims)
	if err := p.complete(ctx, system, answer, "claims", extractSchema, &out); err != nil {
		return nil, err
	}
	claims := make([]Claim, 0, len(out.Claims))
	for _, c := range out.Claims {
		if len(claims) == p.cfg.MaxClaims {
			break
		}
		if strings.TrimSpace(c.Query) == "" || strings.TrimSpace(c.Text) == "" {
			continue
		}
		// A quote the model paraphrased can't be marked in the text; keep the claim, drop the span.
		if !strings.Contains(answer, c.Quote) {
			c.Quote = ""
		}
		if c.Quote == "" || !strings.Contains(c.Quote, c.Value) {
			c.Value = ""
		}
		c.ID = len(claims) + 1
		claims = append(claims, c)
	}
	return claims, nil
}

func (p *Plugin) verify(ctx context.Context, c Claim, now time.Time) (CheckedClaim, Usage) {
	out := CheckedClaim{Claim: c, Status: StatusUnverifiable, Sources: []Source{}}
	var u Usage
	res, searches, err := p.search(ctx, c)
	u.Searches += searches
	if err != nil {
		out.Explanation = "search failed: " + err.Error()
		out.Freshness = Score(out.Status, c.Category, "", 0, now)
		return out, u
	}
	u.Credits += res.Usage.Credits

	var evidence strings.Builder
	fmt.Fprintf(&evidence, "Claim: %s\n\nSearch results:\n", c.Text)
	for i, r := range res.Results {
		official := ""
		if r.Official {
			official = " (official source)"
		}
		fmt.Fprintf(&evidence, "\n[%d]%s %s\nURL: %s\nPublished: %s\n%s\n", i+1, official, r.Title, r.URL, orUnknown(r.PublishedDate), truncate(r.Content, 1500))
	}
	var v struct {
		Status       Status `json:"status"`
		CurrentValue string `json:"current_value"`
		Explanation  string `json:"explanation"`
		SourceIDs    []int  `json:"source_ids"`
		EvidenceDate string `json:"evidence_date"`
	}
	u.ModelCalls++
	if err := p.complete(ctx, fmt.Sprintf(judgeSystem, p.today()), evidence.String(), "verdict", judgeSchema, &v); err != nil {
		out.Explanation = "judge failed: " + err.Error()
		out.Freshness = Score(out.Status, c.Category, "", 0, now)
		return out, u
	}
	out.Status = v.Status
	out.Explanation = v.Explanation
	if v.Status == StatusStale {
		out.CurrentValue = v.CurrentValue
	}
	for _, id := range v.SourceIDs {
		if id < 1 || id > len(res.Results) {
			continue
		}
		r := res.Results[id-1]
		out.Sources = append(out.Sources, Source{Title: r.Title, URL: r.URL, PublishedDate: normalizeDate(r.PublishedDate)})
	}
	out.EvidenceDate = newestDate(normalizeDate(v.EvidenceDate), out.Sources)
	out.Freshness = Score(out.Status, c.Category, out.EvidenceDate, len(out.Sources), now)
	return out, u
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// normalizeDate turns Tavily's RFC 1123 dates and the judge's dates into YYYY-MM-DD.
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// newestDate prefers the sources' own dates; the judge's date (read from page
// text such as "last updated") is used only when no relied-on source is dated.
func newestDate(judged string, sources []Source) string {
	newest := ""
	for _, s := range sources {
		if s.PublishedDate > newest {
			newest = s.PublishedDate
		}
	}
	if newest == "" {
		return judged
	}
	return newest
}
