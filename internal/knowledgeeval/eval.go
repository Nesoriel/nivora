package knowledgeeval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

// Case is one approved-knowledge retrieval benchmark scenario.
type Case struct {
	ID                   string   `json:"id"`
	TenantID             string   `json:"tenant_id"`
	Query                string   `json:"query"`
	ExpectedDocumentIDs  []string `json:"expected_document_ids,omitempty"`
	ForbiddenDocumentIDs []string `json:"forbidden_document_ids,omitempty"`
	MinimumResults       int      `json:"minimum_results,omitempty"`
	MaximumLatencyMS     int64    `json:"maximum_latency_ms,omitempty"`
}

// Observation captures externally visible retrieval behavior.
type Observation struct {
	Items    []knowledge.Item
	Duration time.Duration
	Error    string
}

// Result is JSONL-friendly evaluation output.
type Result struct {
	ID          string   `json:"id"`
	Passed      bool     `json:"passed"`
	Failures    []string `json:"failures,omitempty"`
	DocumentIDs []string `json:"document_ids,omitempty"`
	LatencyMS   int64    `json:"latency_ms"`
	Error       string   `json:"error,omitempty"`
}

// LoadJSONL reads a deterministic retrieval dataset.
func LoadJSONL(reader io.Reader) ([]Case, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	seen := make(map[string]struct{})
	var cases []Case
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var item Case
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("decode knowledge evaluation line %d: %w", lineNumber, err)
		}
		item.ID = strings.TrimSpace(item.ID)
		item.TenantID = strings.TrimSpace(item.TenantID)
		item.Query = strings.TrimSpace(item.Query)
		if item.ID == "" || item.TenantID == "" || item.Query == "" {
			return nil, fmt.Errorf("knowledge evaluation line %d requires id, tenant_id, and query", lineNumber)
		}
		if _, exists := seen[item.ID]; exists {
			return nil, fmt.Errorf("duplicate knowledge evaluation id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read knowledge evaluation dataset: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("knowledge evaluation dataset is empty")
	}
	return cases, nil
}

// Evaluate applies deterministic source and latency assertions.
func Evaluate(item Case, observation Observation) Result {
	result := Result{
		ID:        item.ID,
		Passed:    true,
		LatencyMS: observation.Duration.Milliseconds(),
		Error:     observation.Error,
	}
	seen := make(map[string]struct{}, len(observation.Items))
	for _, knowledgeItem := range observation.Items {
		result.DocumentIDs = append(result.DocumentIDs, knowledgeItem.DocumentID)
		seen[knowledgeItem.DocumentID] = struct{}{}
	}
	if observation.Error != "" {
		result.Failures = append(result.Failures, "retrieval error: "+observation.Error)
	}
	if item.MinimumResults > 0 && len(observation.Items) < item.MinimumResults {
		result.Failures = append(result.Failures, fmt.Sprintf("returned %d results, expected at least %d", len(observation.Items), item.MinimumResults))
	}
	if item.MaximumLatencyMS > 0 && result.LatencyMS > item.MaximumLatencyMS {
		result.Failures = append(result.Failures, fmt.Sprintf("latency %dms exceeded %dms", result.LatencyMS, item.MaximumLatencyMS))
	}
	for _, expected := range item.ExpectedDocumentIDs {
		if _, exists := seen[expected]; !exists {
			result.Failures = append(result.Failures, "missing expected document: "+expected)
		}
	}
	for _, forbidden := range item.ForbiddenDocumentIDs {
		if _, exists := seen[forbidden]; exists {
			result.Failures = append(result.Failures, "returned forbidden document: "+forbidden)
		}
	}
	result.Passed = len(result.Failures) == 0
	return result
}
