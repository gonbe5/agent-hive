package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/chef-guo/agents-hive/internal/llm"
)

type EvidenceSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type EvidenceSource struct {
	URL     string       `json:"url"`
	Tool    string       `json:"tool,omitempty"`
	Title   string       `json:"title,omitempty"`
	Snippet string       `json:"snippet,omitempty"`
	Span    EvidenceSpan `json:"span,omitempty"`
}

type ToolEvidence struct {
	Sources       []EvidenceSource `json:"sources,omitempty"`
	Query         string           `json:"query,omitempty"`
	RawCount      int              `json:"raw_count,omitempty"`
	FilteredCount int              `json:"filtered_count,omitempty"`
	FetchMode     string           `json:"fetch_mode,omitempty"`
}

type GroundingValidator struct {
	NoopMiddleware
}

func BuildToolEvidence(messages []llm.MessageWithTools) ToolEvidence {
	seen := map[string]bool{}
	var out ToolEvidence
	for _, msg := range messages {
		if msg.Role != "tool" {
			continue
		}
		text := msg.Content.Text()
		mergeStructuredToolEvidence(&out, msg.ToolName, text, seen)
		for _, raw := range extractURLs(text) {
			addEvidenceSource(&out, seen, EvidenceSource{
				URL:  raw,
				Tool: msg.ToolName,
			})
		}
	}
	return out
}

func (GroundingValidator) AfterModel(_ context.Context, state *AgentState) error {
	if state == nil || state.Response == nil {
		return nil
	}
	allowed := map[string]bool{}
	for _, src := range state.Evidence.Sources {
		normalized := normalizeEvidenceURL(src.URL)
		if normalized != "" {
			allowed[normalized] = true
		}
	}
	if err := validateCitationClaims(state.Response.Content, allowed); err != nil {
		return err
	}
	for _, raw := range extractURLs(state.Response.Content) {
		normalized := normalizeEvidenceURL(raw)
		if normalized == "" || allowed[normalized] {
			continue
		}
		return fmt.Errorf("grounding validation failed: unverified URL %s", normalized)
	}
	return nil
}

var groundingURLPattern = regexp.MustCompile(`https?://[^\s<>"'，。；、)）\]]+`)
var markdownCitationPattern = regexp.MustCompile(`(?i)\[(citation|source)\s*:[^\]]+\]\((https?://[^)\s]+)\)`)

func extractURLs(text string) []string {
	if text == "" {
		return nil
	}
	return groundingURLPattern.FindAllString(text, -1)
}

func normalizeEvidenceURL(raw string) string {
	raw = cleanEvidenceURL(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Fragment = ""
	u.RawQuery = ""
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

func cleanEvidenceURL(raw string) string {
	raw = strings.TrimSpace(strings.TrimRight(raw, ".,;:!?"))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

func addEvidenceSource(out *ToolEvidence, seen map[string]bool, src EvidenceSource) {
	cleaned := cleanEvidenceURL(src.URL)
	if cleaned == "" || seen[cleaned] {
		return
	}
	seen[cleaned] = true
	src.URL = cleaned
	out.Sources = append(out.Sources, src)
}

func mergeStructuredToolEvidence(out *ToolEvidence, toolName, text string, seen map[string]bool) {
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return
	}
	collectStructuredToolEvidence(out, toolName, payload, seen)
}

func collectStructuredToolEvidence(out *ToolEvidence, toolName string, value any, seen map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		if out.Query == "" {
			out.Query = stringField(v, "query")
		}
		if out.RawCount == 0 {
			out.RawCount = intField(v, "raw_count", "rawCount")
		}
		if out.FilteredCount == 0 {
			out.FilteredCount = intField(v, "filtered_count", "filteredCount")
		}
		if out.FetchMode == "" {
			out.FetchMode = firstStringField(v, "fetch_mode", "fetchMode")
		}
		if src := evidenceSourceFromMap(v, toolName); src.URL != "" {
			addEvidenceSource(out, seen, src)
		}
		for _, child := range v {
			collectStructuredToolEvidence(out, toolName, child, seen)
		}
	case []any:
		for _, child := range v {
			collectStructuredToolEvidence(out, toolName, child, seen)
		}
	}
}

func evidenceSourceFromMap(v map[string]any, toolName string) EvidenceSource {
	src := EvidenceSource{
		URL:     firstStringField(v, "url", "source_url", "sourceURL", "href"),
		Tool:    toolName,
		Title:   firstStringField(v, "title", "name"),
		Snippet: firstStringField(v, "snippet", "description", "text"),
	}
	if span, ok := evidenceSpanFromValue(v["span"]); ok {
		src.Span = span
	} else if span, ok := evidenceSpanFromValue(v["source_span"]); ok {
		src.Span = span
	} else if span, ok := evidenceSpanFromValue(v["sourceSpan"]); ok {
		src.Span = span
	}
	return src
}

func validateCitationClaims(text string, allowed map[string]bool) error {
	if len(markdownCitationPattern.FindAllStringSubmatch(text, -1)) > 0 && len(allowed) == 0 {
		return fmt.Errorf("grounding validation failed: citation without evidence")
	}
	for _, match := range markdownCitationPattern.FindAllStringSubmatch(text, -1) {
		normalized := normalizeEvidenceURL(match[2])
		if normalized != "" && !allowed[normalized] {
			return fmt.Errorf("grounding validation failed: citation without evidence %s", normalized)
		}
	}

	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil
	}
	for _, claim := range collectCitationClaims(payload, false) {
		if claim.spanPresent && !validEvidenceSpan(claim.span) {
			return fmt.Errorf("grounding validation failed: invalid source span")
		}
		if claim.missingURL || normalizeEvidenceURL(claim.url) == "" {
			return fmt.Errorf("grounding validation failed: citation without evidence")
		}
		if normalized := normalizeEvidenceURL(claim.url); !allowed[normalized] {
			return fmt.Errorf("grounding validation failed: citation without evidence %s", normalized)
		}
	}
	return nil
}

type citationClaim struct {
	url         string
	span        EvidenceSpan
	spanPresent bool
	missingURL  bool
}

func collectCitationClaims(value any, inCitationList bool) []citationClaim {
	var claims []citationClaim
	switch v := value.(type) {
	case map[string]any:
		isClaim := inCitationList || hasAnyKey(v, "source_span", "sourceSpan")
		if isClaim {
			claim := citationClaim{url: firstStringField(v, "url", "source_url", "sourceURL", "href")}
			if rawSpan, ok := firstValue(v, "source_span", "sourceSpan", "span"); ok {
				claim.spanPresent = true
				claim.span, _ = evidenceSpanFromValue(rawSpan)
			}
			if claim.url == "" {
				claim.missingURL = true
			}
			claims = append(claims, claim)
		}
		for key, child := range v {
			claims = append(claims, collectCitationClaims(child, isCitationListKey(key))...)
		}
	case []any:
		for _, child := range v {
			claims = append(claims, collectCitationClaims(child, inCitationList)...)
		}
	}
	return claims
}

func isCitationListKey(key string) bool {
	switch strings.ToLower(key) {
	case "citations", "sources", "source_spans", "sourcespans":
		return true
	default:
		return false
	}
}

func validEvidenceSpan(span EvidenceSpan) bool {
	return span.Start >= 0 && span.End > span.Start
}

func evidenceSpanFromValue(value any) (EvidenceSpan, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return EvidenceSpan{}, false
	}
	start, okStart := intValue(m["start"])
	end, okEnd := intValue(m["end"])
	if !okStart || !okEnd {
		return EvidenceSpan{}, false
	}
	return EvidenceSpan{Start: start, End: end}, true
}

func stringField(v map[string]any, key string) string {
	s, _ := v[key].(string)
	return s
}

func firstStringField(v map[string]any, keys ...string) string {
	raw, ok := firstValue(v, keys...)
	if !ok {
		return ""
	}
	s, _ := raw.(string)
	return s
}

func firstValue(v map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if raw, ok := v[key]; ok {
			return raw, true
		}
	}
	return nil, false
}

func intField(v map[string]any, keys ...string) int {
	raw, ok := firstValue(v, keys...)
	if !ok {
		return 0
	}
	n, ok := intValue(raw)
	if !ok {
		return 0
	}
	return n
}

func intValue(raw any) (int, bool) {
	switch v := raw.(type) {
	case float64:
		return int(v), v == float64(int(v))
	case int:
		return v, true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

func hasAnyKey(v map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := v[key]; ok {
			return true
		}
	}
	return false
}
