package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/Nesoriel/nivora/internal/eval"
)

// ShadowResult compares externally observable baseline and candidate behavior.
type ShadowResult struct {
	ID                    string   `json:"id"`
	CandidatePassed       bool     `json:"candidate_passed"`
	CandidateFailures     []string `json:"candidate_failures,omitempty"`
	BaselineCompleted     bool     `json:"baseline_completed"`
	CandidateCompleted    bool     `json:"candidate_completed"`
	BaselineErrorCode     string   `json:"baseline_error_code,omitempty"`
	CandidateErrorCode    string   `json:"candidate_error_code,omitempty"`
	BaselineAnswerSHA256  string   `json:"baseline_answer_sha256"`
	CandidateAnswerSHA256 string   `json:"candidate_answer_sha256"`
	BaselineAnswerBytes   int      `json:"baseline_answer_bytes"`
	CandidateAnswerBytes  int      `json:"candidate_answer_bytes"`
	BaselineTools         []string `json:"baseline_tools,omitempty"`
	CandidateTools        []string `json:"candidate_tools,omitempty"`
	ToolSetsEqual         bool     `json:"tool_sets_equal"`
	BaselineFirstTokenMS  int64    `json:"baseline_first_token_ms"`
	CandidateFirstTokenMS int64    `json:"candidate_first_token_ms"`
	BaselineDurationMS    int64    `json:"baseline_duration_ms"`
	CandidateDurationMS   int64    `json:"candidate_duration_ms"`
}

// CompareShadow evaluates the candidate against deterministic expectations and
// compares it with a baseline without persisting answer text.
func CompareShadow(item eval.Case, baseline, candidate eval.Observation) ShadowResult {
	candidateEvaluation := eval.Evaluate(item, candidate)
	baselineTools := normalizedTools(baseline.Tools)
	candidateTools := normalizedTools(candidate.Tools)
	return ShadowResult{
		ID:                    item.ID,
		CandidatePassed:       candidateEvaluation.Passed,
		CandidateFailures:     candidateEvaluation.Failures,
		BaselineCompleted:     baseline.Completed,
		CandidateCompleted:    candidate.Completed,
		BaselineErrorCode:     baseline.ErrorCode,
		CandidateErrorCode:    candidate.ErrorCode,
		BaselineAnswerSHA256:  answerHash(baseline.Answer),
		CandidateAnswerSHA256: answerHash(candidate.Answer),
		BaselineAnswerBytes:   len([]byte(baseline.Answer)),
		CandidateAnswerBytes:  len([]byte(candidate.Answer)),
		BaselineTools:         baselineTools,
		CandidateTools:        candidateTools,
		ToolSetsEqual:         equalStrings(baselineTools, candidateTools),
		BaselineFirstTokenMS:  baseline.FirstToken.Milliseconds(),
		CandidateFirstTokenMS: candidate.FirstToken.Milliseconds(),
		BaselineDurationMS:    baseline.Duration.Milliseconds(),
		CandidateDurationMS:   candidate.Duration.Milliseconds(),
	}
}

func answerHash(answer string) string {
	digest := sha256.Sum256([]byte(answer))
	return hex.EncodeToString(digest[:])
}

func normalizedTools(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
