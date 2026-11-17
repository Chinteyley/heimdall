package heimdall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// internalHeader marks Heimdall's own model calls so the plugin lets them pass unchecked.
const internalHeader = "x-heimdall-internal"

var httpClient = &http.Client{}

func postJSON(ctx context.Context, url string, headers map[string]string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s: HTTP %d: %s", url, resp.StatusCode, truncate(string(data), 300))
	}
	return json.Unmarshal(data, out)
}

// tavilyResult is the part of a Tavily search result Heimdall reads.
type tavilyResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
	PublishedDate string  `json:"published_date"`
	Official      bool    `json:"-"` // from the claim's official site
}

type tavilySearch struct {
	Results []tavilyResult `json:"results"`
	Usage   struct {
		Credits int `json:"credits"`
	} `json:"usage"`
}

// search runs the claim's query on the open web and, when the extractor named
// an official site, on that site alone too, in parallel. Official results come
// first so the judge sees the primary source even when the open web ranks a
// copy of it higher.
func (p *Plugin) search(ctx context.Context, c Claim) (tavilySearch, int, error) {
	topic := "general"
	if c.Category == CategoryLeadership || c.Category == CategoryCompany {
		topic = "news"
	}
	domain := cleanDomain(c.Domain)
	var official, open tavilySearch
	var officialErr, openErr error
	var wg sync.WaitGroup
	if domain != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			official, officialErr = p.tavily(ctx, c.Query, "general", []string{domain})
		}()
	}
	open, openErr = p.tavily(ctx, c.Query, topic, nil)
	wg.Wait()
	searches := 1
	if domain != "" {
		searches = 2
	}
	if openErr != nil && (domain == "" || officialErr != nil) {
		return tavilySearch{}, searches, openErr
	}

	merged := tavilySearch{}
	merged.Usage.Credits = official.Usage.Credits + open.Usage.Credits
	seen := map[string]bool{}
	for _, r := range official.Results {
		r.Official = true
		seen[r.URL] = true
		merged.Results = append(merged.Results, r)
	}
	for _, r := range open.Results {
		if !seen[r.URL] {
			r.Official = domain != "" && hostMatches(r.URL, domain)
			merged.Results = append(merged.Results, r)
		}
	}
	return merged, searches, nil
}

func (p *Plugin) tavily(ctx context.Context, query, topic string, domains []string) (tavilySearch, error) {
	body := map[string]any{
		"query":                  query,
		"topic":                  topic,
		"search_depth":           p.cfg.SearchDepth,
		"max_results":            p.cfg.MaxResults,
		"include_published_date": true,
		"include_usage":          true,
	}
	if len(domains) > 0 {
		body["include_domains"] = domains
	}
	var out tavilySearch
	err := postJSON(ctx, p.cfg.TavilyURL+"/search", map[string]string{"Authorization": "Bearer " + p.cfg.TavilyAPIKey}, body, &out)
	return out, err
}

func hostMatches(rawURL, domain string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == domain || strings.HasSuffix(h, "."+domain)
}

func cleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d, _, _ = strings.Cut(d, "/")
	if !strings.Contains(d, ".") || strings.ContainsAny(d, " \t") {
		return ""
	}
	return d
}

// complete asks the check model, through this same Bifrost, for JSON matching schema.
func (p *Plugin) complete(ctx context.Context, system, user, schemaName string, schema map[string]any, out any) error {
	headers := map[string]string{internalHeader: p.token}
	for k, v := range p.cfg.GatewayHeaders {
		headers[k] = v
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err := postJSON(ctx, p.cfg.GatewayURL+"/v1/chat/completions", headers, map[string]any{
		"model": p.cfg.CheckModel,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": schemaName, "strict": true, "schema": schema},
		},
	}, &resp)
	if err != nil {
		return err
	}
	if len(resp.Choices) == 0 {
		return fmt.Errorf("check model returned no choices")
	}
	return json.Unmarshal([]byte(resp.Choices[0].Message.Content), out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
