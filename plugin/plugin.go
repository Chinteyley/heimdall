// Package heimdall is a Bifrost plugin that checks LLM answers for claims that
// have gone stale (prices, versions, who runs what, laws) against live Tavily
// search, scores each claim's freshness, and can annotate or correct the answer.
package heimdall

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

// ModeHeader picks the mode per request; it is removed before the provider sees it.
const ModeHeader = "x-heimdall"

const modeKey schemas.BifrostContextKey = "heimdall-mode"

// Plugin holds the parsed config. now is swappable for tests.
type Plugin struct {
	cfg Config
	now func() time.Time
	// token marks Heimdall's own calls back into Bifrost. It is random per
	// process, so a client cannot send it to skip a check the operator enabled.
	token string
}

// New builds a plugin from the raw config Bifrost passes to Init.
func New(raw any) (*Plugin, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &Plugin{cfg: cfg, now: time.Now, token: hex.EncodeToString(b)}, nil
}

func (p *Plugin) today() string { return p.now().UTC().Format("2006-01-02") }

func (p *Plugin) GetName() string { return "heimdall" }

func (p *Plugin) Cleanup() error { return nil }

// HTTPTransportPreHook decides the mode. In check mode it answers the request
// itself with a report on the last user message, without calling a model.
func (p *Plugin) HTTPTransportPreHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest) (*schemas.HTTPResponse, error) {
	if req.Method != "POST" || !slices.Contains(p.cfg.Paths, req.Path) {
		return nil, nil
	}
	internal := req.CaseInsensitiveHeaderLookup(internalHeader)
	deleteHeader(req.Headers, internalHeader)
	if internal != "" && subtle.ConstantTimeCompare([]byte(internal), []byte(p.token)) == 1 {
		return nil, nil
	}
	mode := p.cfg.DefaultMode
	if h := req.CaseInsensitiveHeaderLookup(ModeHeader); h != "" {
		m, ok := parseMode(strings.ToLower(strings.TrimSpace(h)))
		if !ok {
			return jsonResponse(400, map[string]any{"error": map[string]string{
				"message": "x-heimdall must be one of off, report, annotate, correct, check", "type": "invalid_request_error"}}), nil
		}
		mode = m
	}
	deleteHeader(req.Headers, ModeHeader)
	if mode == ModeOff {
		return nil, nil
	}

	var body struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, nil // not ours to validate; Bifrost will reject it
	}

	if mode == ModeCheck {
		answer := ""
		for i := len(body.Messages) - 1; i >= 0; i-- {
			if body.Messages[i].Role == "user" {
				answer = messageText(body.Messages[i].Content)
				break
			}
		}
		if strings.TrimSpace(answer) == "" {
			return jsonResponse(400, map[string]any{"error": map[string]string{
				"message": "check mode needs the answer to check as the last user message", "type": "invalid_request_error"}}), nil
		}
		rep := p.Check(detach(ctx), answer, ModeCheck)
		resp := jsonResponse(200, map[string]any{"object": "heimdall.check", "heimdall": rep})
		setReportHeaders(resp.Headers, rep)
		return resp, nil
	}

	// A stream is on the wire before it ends, so there is nothing left to rewrite.
	if body.Stream {
		return nil, nil
	}
	ctx.SetValue(modeKey, mode)
	return nil, nil
}

// HTTPTransportPostHook checks the assistant's answer and rewrites the body.
func (p *Plugin) HTTPTransportPostHook(ctx *schemas.BifrostContext, req *schemas.HTTPRequest, resp *schemas.HTTPResponse) error {
	mode, _ := ctx.Value(modeKey).(Mode)
	if mode == "" || mode == ModeOff || resp.StatusCode != 200 {
		return nil
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil
	}
	msg, answer, ok := firstMessage(body)
	if !ok || strings.TrimSpace(answer) == "" {
		return nil
	}

	rep := p.Check(detach(ctx), answer, mode)
	if rep.Error == "" {
		switch mode {
		case ModeAnnotate:
			msg["content"] = Annotate(answer, rep)
		case ModeCorrect:
			fixed, err := p.correct(detach(ctx), answer, &rep)
			if err != nil {
				rep.Error = "correct: " + err.Error()
			} else {
				msg["content"] = fixed
			}
		}
	}
	body["heimdall"] = rep
	out, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	resp.Body = out
	setReportHeaders(resp.Headers, rep)
	return nil
}

// detach keeps the request's values but not its cancellation, so a check is
// bounded by its own timeout rather than by transport internals.
func detach(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }

func firstMessage(body map[string]any) (map[string]any, string, bool) {
	choices, _ := body["choices"].([]any)
	if len(choices) == 0 {
		return nil, "", false
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	content, ok := msg["content"].(string)
	return msg, content, ok
}

// messageText reads an OpenAI message content: a string or an array of text parts.
func messageText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

func jsonResponse(status int, v any) *schemas.HTTPResponse {
	b, _ := json.Marshal(v)
	return &schemas.HTTPResponse{StatusCode: status, Headers: map[string]string{"Content-Type": "application/json"}, Body: b}
}

func setReportHeaders(h map[string]string, rep Report) {
	if rep.Error != "" {
		h["x-heimdall-error"] = truncate(rep.Error, 200)
		return
	}
	h["x-heimdall-score"] = strconv.Itoa(rep.Score)
	h["x-heimdall-stale"] = strconv.Itoa(rep.Stale)
	h["x-heimdall-claims"] = strconv.Itoa(len(rep.Claims))
}

func deleteHeader(h map[string]string, name string) {
	for k := range h {
		if strings.EqualFold(k, name) {
			delete(h, k)
		}
	}
}
