package acceptance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeCase is one deterministic HTTP security or protocol scenario.
type ProbeCase struct {
	ID                  string            `json:"id"`
	Method              string            `json:"method,omitempty"`
	Path                string            `json:"path"`
	ServiceKey          string            `json:"service_key,omitempty"`
	Bearer              string            `json:"bearer,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	Body                json.RawMessage   `json:"body,omitempty"`
	ExpectedStatus      int               `json:"expected_status"`
	RequiredSubstrings  []string          `json:"required_substrings,omitempty"`
	ForbiddenSubstrings []string          `json:"forbidden_substrings,omitempty"`
	MaximumLatencyMS    int64             `json:"maximum_latency_ms,omitempty"`
}

// ProbeResult is JSONL-friendly and contains no configured secrets.
type ProbeResult struct {
	ID        string   `json:"id"`
	Passed    bool     `json:"passed"`
	Status    int      `json:"status"`
	LatencyMS int64    `json:"latency_ms"`
	Failures  []string `json:"failures,omitempty"`
}

// ProbeClient runs deterministic HTTP probes.
type ProbeClient struct {
	BaseURL      string
	SharedSecret string
	BearerToken  string
	HTTPClient   *http.Client
}

// LoadProbeJSONL reads probe scenarios.
func LoadProbeJSONL(reader io.Reader) ([]ProbeCase, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	seen := make(map[string]struct{})
	var cases []ProbeCase
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var item ProbeCase
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode probe line %d: %w", lineNumber, err)
		}
		item.ID = strings.TrimSpace(item.ID)
		item.Path = strings.TrimSpace(item.Path)
		if item.ID == "" || item.Path == "" || item.ExpectedStatus == 0 {
			return nil, fmt.Errorf("probe line %d requires id, path, and expected_status", lineNumber)
		}
		if _, exists := seen[item.ID]; exists {
			return nil, fmt.Errorf("duplicate probe id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("probe dataset is empty")
	}
	return cases, nil
}

// Run executes one probe without returning response bodies to result output.
func (c ProbeClient) Run(ctx context.Context, item ProbeCase) (ProbeResult, error) {
	method := strings.TrimSpace(item.Method)
	if method == "" {
		method = http.MethodPost
	}
	var body io.Reader
	if len(item.Body) > 0 {
		body = bytes.NewReader(item.Body)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+item.Path, body)
	if err != nil {
		return ProbeResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	switch strings.ToLower(strings.TrimSpace(item.ServiceKey)) {
	case "missing":
	case "invalid":
		request.Header.Set("X-Nivora-Key", "invalid-probe-key")
	default:
		request.Header.Set("X-Nivora-Key", c.SharedSecret)
	}
	switch strings.ToLower(strings.TrimSpace(item.Bearer)) {
	case "missing":
	case "invalid":
		request.Header.Set("Authorization", "Bearer invalid-probe-context")
	case "valid":
		request.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}
	for key, value := range item.Headers {
		request.Header.Set(key, value)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return ProbeResult{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return ProbeResult{}, err
	}
	content := strings.ToLower(string(raw))
	result := ProbeResult{ID: item.ID, Passed: true, Status: response.StatusCode, LatencyMS: latency.Milliseconds()}
	if response.StatusCode != item.ExpectedStatus {
		result.Failures = append(result.Failures, fmt.Sprintf("status %d, expected %d", response.StatusCode, item.ExpectedStatus))
	}
	if item.MaximumLatencyMS > 0 && result.LatencyMS > item.MaximumLatencyMS {
		result.Failures = append(result.Failures, fmt.Sprintf("latency %dms exceeded %dms", result.LatencyMS, item.MaximumLatencyMS))
	}
	for _, required := range item.RequiredSubstrings {
		if !strings.Contains(content, strings.ToLower(required)) {
			result.Failures = append(result.Failures, "missing required substring: "+required)
		}
	}
	for _, forbidden := range item.ForbiddenSubstrings {
		if strings.Contains(content, strings.ToLower(forbidden)) {
			result.Failures = append(result.Failures, "contained forbidden substring: "+forbidden)
		}
	}
	result.Passed = len(result.Failures) == 0
	return result, nil
}
