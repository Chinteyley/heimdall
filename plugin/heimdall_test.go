package heimdall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

var testNow = time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

const staleAnswer = "GPT-4o costs $5.00 per million input tokens, and the latest Next.js is 15."

func TestScore(t *testing.T) {
	cases := []struct {
		name    string
		status  Status
		cat     Category
		date    string
		sources int
		want    int
	}{
		{"current, dated today, three sources", StatusCurrent, CategoryPricing, "2026-11-17", 3, 100},
		{"current, past the horizon, one source", StatusCurrent, CategoryPricing, "2026-01-01", 1, 65},
		{"current, undated", StatusCurrent, CategoryOther, "", 0, 73},
		{"stale, fresh contradiction", StatusStale, CategoryVersion, "2026-11-17", 2, 0},
		{"stale, old contradiction", StatusStale, CategoryVersion, "2025-01-01", 1, 15},
		{"unverifiable price", StatusUnverifiable, CategoryPricing, "", 0, 40},
		{"unverifiable leadership", StatusUnverifiable, CategoryLeadership, "", 0, 50},
	}
	for _, c := range cases {
		if got := Score(c.status, c.cat, c.date, c.sources, testNow); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestAnnotateStrikesStaleValueInline(t *testing.T) {
	rep := Report{CheckedAt: testNow, Stale: 1, Claims: []CheckedClaim{{
		Claim:        Claim{Text: "GPT-4o input price", Quote: "GPT-4o costs $5.00 per million", Value: "$5.00"},
		Status:       StatusStale,
		CurrentValue: "$2.50",
		Explanation:  "OpenAI cut the price.",
		Sources:      []Source{{Title: "Pricing", URL: "https://openai.com/api/pricing", PublishedDate: "2026-10-01"}},
	}}}
	got := Annotate(staleAnswer, rep)
	if !strings.Contains(got, "GPT-4o costs ~~$5.00~~ **$2.50** [1] per million") {
		t.Fatalf("value not struck inline:\n%s", got)
	}
	if !strings.Contains(got, "[1] OpenAI cut the price. ([Pricing](https://openai.com/api/pricing), 2026-10-01)") {
		t.Fatalf("footnote missing:\n%s", got)
	}
	if Annotate(staleAnswer, Report{}) != staleAnswer {
		t.Fatal("an answer with no claims must be left as it is")
	}
}

// fakes stands in for Tavily and for Bifrost's own chat endpoint.
type fakes struct {
	gateway, tavily  *httptest.Server
	internalCalls    atomic.Int32
	searches         atomic.Int32
	officialSearches atomic.Int32
}

func newFakes(t *testing.T) *fakes {
	f := &fakes{}
	f.gateway = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(internalHeader) == "" {
			t.Errorf("check model call is missing %s", internalHeader)
		}
		f.internalCalls.Add(1)
		var req struct {
			ResponseFormat struct {
				JSONSchema struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var content any
		switch req.ResponseFormat.JSONSchema.Name {
		case "claims":
			content = map[string]any{"claims": []map[string]string{
				{"text": "GPT-4o costs $5.00 per million input tokens", "quote": "GPT-4o costs $5.00 per million input tokens", "value": "$5.00", "category": "pricing", "query": "GPT-4o API input price", "domain": "https://openai.com/api/pricing"},
				{"text": "The latest Next.js major version is 15", "quote": "the latest Next.js is 15", "value": "15", "category": "version", "query": "latest Next.js version", "domain": ""},
			}}
		case "verdict":
			if strings.Contains(req.Messages[1].Content, "GPT-4o") {
				content = map[string]any{"status": "stale", "current_value": "$2.50", "explanation": "Price cut to $2.50.", "source_ids": []int{1}, "evidence_date": ""}
			} else {
				content = map[string]any{"status": "current", "current_value": "", "explanation": "Still 15.", "source_ids": []int{1, 2}, "evidence_date": ""}
			}
		case "corrected":
			content = map[string]any{"answer": "GPT-4o costs $2.50 [1] per million input tokens."}
		}
		b, _ := json.Marshal(content)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": string(b)}}}})
	}))
	f.tavily = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tvly-test" {
			t.Errorf("tavily auth header = %q", r.Header.Get("Authorization"))
		}
		f.searches.Add(1)
		var q map[string]any
		_ = json.NewDecoder(r.Body).Decode(&q)
		if d, ok := q["include_domains"].([]any); ok {
			f.officialSearches.Add(1)
			if len(d) != 1 || d[0] != "openai.com" {
				t.Errorf("official search on the wrong domain: %v", d)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "Pricing", "url": "https://example.com/pricing", "content": "...", "published_date": "Mon, 16 Nov 2026 00:00:00 GMT"},
				{"title": "Docs", "url": "https://example.com/docs", "content": "...", "published_date": nil},
			},
			"usage": map[string]int{"credits": 1},
		})
	}))
	t.Cleanup(func() { f.gateway.Close(); f.tavily.Close() })
	return f
}

func (f *fakes) plugin(t *testing.T) *Plugin {
	p, err := New(map[string]any{"tavily_api_key": "tvly-test", "tavily_url": f.tavily.URL, "gateway_url": f.gateway.URL})
	if err != nil {
		t.Fatal(err)
	}
	p.now = func() time.Time { return testNow }
	return p
}

func chatRequest(mode string, messages ...string) *schemas.HTTPRequest {
	msgs := make([]map[string]string, len(messages))
	for i, m := range messages {
		msgs[i] = map[string]string{"role": "user", "content": m}
	}
	body, _ := json.Marshal(map[string]any{"model": "openai/gpt-5.4", "messages": msgs})
	return &schemas.HTTPRequest{Method: "POST", Path: "/v1/chat/completions", Headers: map[string]string{"X-Heimdall": mode}, Body: body}
}

func chatResponse(content string) *schemas.HTTPResponse {
	body, _ := json.Marshal(map[string]any{"id": "c1", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": content}}}})
	return &schemas.HTTPResponse{StatusCode: 200, Headers: map[string]string{}, Body: body}
}

func TestAnnotateModeRewritesResponse(t *testing.T) {
	f := newFakes(t)
	p := f.plugin(t)
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	req := chatRequest("annotate", "What does GPT-4o cost?")

	if resp, err := p.HTTPTransportPreHook(ctx, req); resp != nil || err != nil {
		t.Fatalf("pre-hook should pass through, got %v %v", resp, err)
	}
	if _, ok := req.Headers["X-Heimdall"]; ok {
		t.Fatal("x-heimdall must not reach the provider")
	}
	resp := chatResponse(staleAnswer)
	if err := p.HTTPTransportPostHook(ctx, req, resp); err != nil {
		t.Fatal(err)
	}

	var body struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Heimdall Report `json:"heimdall"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatal(err)
	}
	rep := body.Heimdall
	if rep.Error != "" || len(rep.Claims) != 2 || rep.Stale != 1 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Claims[0].Freshness != 0 || rep.Claims[0].EvidenceDate != "2026-11-16" {
		t.Fatalf("stale claim scored %d on %q", rep.Claims[0].Freshness, rep.Claims[0].EvidenceDate)
	}
	if rep.Score != 0 || resp.Headers["x-heimdall-score"] != "0" || resp.Headers["x-heimdall-stale"] != "1" {
		t.Fatalf("score %d, headers %v", rep.Score, resp.Headers)
	}
	if !strings.Contains(body.Choices[0].Message.Content, "~~$5.00~~ **$2.50** [1]") {
		t.Fatalf("content not annotated: %s", body.Choices[0].Message.Content)
	}
	// Two claims, one with an official domain: two open searches plus one on openai.com.
	if f.searches.Load() != 3 || f.officialSearches.Load() != 1 || f.internalCalls.Load() != 3 || rep.Usage.Searches != 3 {
		t.Fatalf("searches %d (official %d, reported %d), model calls %d", f.searches.Load(), f.officialSearches.Load(), rep.Usage.Searches, f.internalCalls.Load())
	}
}

func TestCorrectModeRewritesAnswer(t *testing.T) {
	f := newFakes(t)
	p := f.plugin(t)
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})
	req := chatRequest("correct", "What does GPT-4o cost?")
	_, _ = p.HTTPTransportPreHook(ctx, req)
	resp := chatResponse(staleAnswer)
	_ = p.HTTPTransportPostHook(ctx, req, resp)
	if !strings.Contains(string(resp.Body), "GPT-4o costs $2.50 [1] per million input tokens.") {
		t.Fatalf("answer not corrected: %s", resp.Body)
	}
}

func TestCheckModeAnswersWithoutAModel(t *testing.T) {
	f := newFakes(t)
	p := f.plugin(t)
	resp, err := p.HTTPTransportPreHook(schemas.NewBifrostContext(context.Background(), time.Time{}), chatRequest("check", staleAnswer))
	if err != nil || resp == nil || resp.StatusCode != 200 {
		t.Fatalf("check mode must short-circuit with 200, got %+v %v", resp, err)
	}
	var body struct {
		Object   string `json:"object"`
		Heimdall Report `json:"heimdall"`
	}
	_ = json.Unmarshal(resp.Body, &body)
	if body.Object != "heimdall.check" || len(body.Heimdall.Claims) != 2 {
		t.Fatalf("body = %s", resp.Body)
	}
}

func TestPassesThroughWhatItShouldNotTouch(t *testing.T) {
	f := newFakes(t)
	p := f.plugin(t)
	ctx := schemas.NewBifrostContext(context.Background(), time.Time{})

	internal := chatRequest("annotate", "hi")
	internal.Headers[internalHeader] = p.token
	if resp, _ := p.HTTPTransportPreHook(ctx, internal); resp != nil || ctx.Value(modeKey) != nil {
		t.Fatal("Heimdall's own calls must not be checked")
	}
	forged := chatRequest("annotate", "hi")
	forged.Headers[internalHeader] = "1"
	forgedCtx := schemas.NewBifrostContext(context.Background(), time.Time{})
	if _, _ = p.HTTPTransportPreHook(forgedCtx, forged); forgedCtx.Value(modeKey) != ModeAnnotate {
		t.Fatal("a forged internal header must not skip the check")
	}
	if resp, _ := p.HTTPTransportPreHook(ctx, chatRequest("off", "hi")); resp != nil || ctx.Value(modeKey) != nil {
		t.Fatal("off must not check")
	}
	if resp, _ := p.HTTPTransportPreHook(ctx, chatRequest("loud", "hi")); resp == nil || resp.StatusCode != 400 {
		t.Fatal("an unknown mode must be rejected")
	}
	if f.internalCalls.Load() != 0 || f.searches.Load() != 0 {
		t.Fatal("nothing should have been called")
	}
}
