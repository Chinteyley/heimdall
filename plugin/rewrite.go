package heimdall

import (
	"context"
	"fmt"
	"strings"
)

// Annotate strikes each stale value through inline, writes the current value
// beside it with a numbered note, and lists the notes and sources at the end.
func Annotate(answer string, rep Report) string {
	if len(rep.Claims) == 0 {
		return answer
	}
	var notes []string
	for _, c := range rep.Claims {
		if c.Status != StatusStale {
			continue
		}
		n := len(notes) + 1
		marked := false
		if c.Quote != "" && c.Value != "" && c.CurrentValue != "" {
			fixed := strings.Replace(c.Quote, c.Value, fmt.Sprintf("~~%s~~ **%s** [%d]", c.Value, c.CurrentValue, n), 1)
			if i := strings.Index(answer, c.Quote); i >= 0 {
				answer = answer[:i] + fixed + answer[i+len(c.Quote):]
				marked = true
			}
		}
		note := fmt.Sprintf("[%d] %s", n, c.Explanation)
		if !marked {
			note = fmt.Sprintf("[%d] Out of date: %q. %s", n, c.Text, c.Explanation)
		}
		if len(c.Sources) > 0 {
			note += " " + sourceLink(c.Sources[0])
		}
		notes = append(notes, note)
	}
	return answer + "\n\n---\n\n" + summaryLine(rep) + strings.Join(prefixAll(notes, "\n\n"), "")
}

func summaryLine(rep Report) string {
	return fmt.Sprintf("*Checked by Heimdall on %s: %d time-sensitive claim%s, %d out of date.*",
		rep.CheckedAt.Format("Jan 2, 2006"), len(rep.Claims), plural(len(rep.Claims)), rep.Stale)
}

func sourceLink(s Source) string {
	if s.PublishedDate != "" {
		return fmt.Sprintf("([%s](%s), %s)", s.Title, s.URL, s.PublishedDate)
	}
	return fmt.Sprintf("([%s](%s))", s.Title, s.URL)
}

func prefixAll(lines []string, prefix string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = prefix + l
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

const correctSystem = `You correct an AI assistant's answer using a fact-check report.
Rewrite the answer so every claim marked stale states its current value instead, with a citation like [1] after it that points to the listed source.
Also update any figure the answer computed from a corrected value (a unit conversion, a total, a comparison) so the answer stays consistent.
Keep everything else as it was: wording, structure, formatting, tone and length. Do not add facts that are not in the report.
End with a "Sources" list of the cited links, numbered to match.`

var correctSchema = map[string]any{
	"type": "object", "additionalProperties": false, "required": []string{"answer"},
	"properties": map[string]any{"answer": map[string]any{"type": "string"}},
}

// correct rewrites the answer from the report's stale verdicts through the check model.
func (p *Plugin) correct(ctx context.Context, answer string, rep *Report) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Answer:\n%s\n\nFact-check report (today is %s):\n", answer, p.today())
	n := 0
	for _, c := range rep.Claims {
		if c.Status != StatusStale {
			continue
		}
		n++
		fmt.Fprintf(&b, "\n%d. Stale: %q. Now: %q. %s", n, c.Text, c.CurrentValue, c.Explanation)
		if len(c.Sources) > 0 {
			fmt.Fprintf(&b, " Source: %s (%s)", c.Sources[0].Title, c.Sources[0].URL)
		}
	}
	if n == 0 {
		return answer, nil
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	defer cancel()
	var out struct {
		Answer string `json:"answer"`
	}
	rep.Usage.ModelCalls++
	if err := p.complete(ctx, correctSystem, b.String(), "corrected", correctSchema, &out); err != nil {
		return answer, err
	}
	return out.Answer, nil
}
