package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
)

// Case is one synthetic customer-support regression scenario.
type Case struct {
	ID        string               `json:"id"`
	Question  string               `json:"question"`
	History   []domain.Turn        `json:"history,omitempty"`
	Tenant    domain.TenantContext `json:"tenant"`
	Principal domain.Principal     `json:"principal"`
	Expected  Expectations         `json:"expected"`
}

// Expectations describe externally observable behavior, never private chain of thought.
type Expectations struct {
	RequiredSubstrings  []string `json:"required_substrings,omitempty"`
	ForbiddenSubstrings []string `json:"forbidden_substrings,omitempty"`
	RequiredTools       []string `json:"required_tools,omitempty"`
	ForbiddenTools      []string `json:"forbidden_tools,omitempty"`
	MaxLatencyMS        int64    `json:"max_latency_ms,omitempty"`
	AllowAgentError     bool     `json:"allow_agent_error,omitempty"`
}

// Observation is collected from one Nivora SSE run.
type Observation struct {
	Answer     string        `json:"answer"`
	Tools      []string      `json:"tools"`
	Completed  bool          `json:"completed"`
	ErrorCode  string        `json:"error_code,omitempty"`
	FirstToken time.Duration `json:"-"`
	Duration   time.Duration `json:"-"`
}

// Result is a JSONL-friendly evaluation output.
type Result struct {
	ID         string   `json:"id"`
	Passed     bool     `json:"passed"`
	Failures   []string `json:"failures,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	Answer     string   `json:"answer"`
	Tools      []string `json:"tools"`
	ErrorCode  string   `json:"error_code,omitempty"`
}

// LoadJSONL reads a deterministic evaluation dataset.
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
			return nil, fmt.Errorf("decode evaluation line %d: %w", lineNumber, err)
		}
		item.ID = strings.TrimSpace(item.ID)
		item.Question = strings.TrimSpace(item.Question)
		if item.ID == "" || item.Question == "" || strings.TrimSpace(item.Tenant.ID) == "" {
			return nil, fmt.Errorf("evaluation line %d requires id, question, and tenant.id", lineNumber)
		}
		if _, exists := seen[item.ID]; exists {
			return nil, fmt.Errorf("duplicate evaluation id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		cases = append(cases, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read evaluation dataset: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("evaluation dataset is empty")
	}
	return cases, nil
}
