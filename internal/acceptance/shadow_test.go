package acceptance

import (
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/eval"
)

func TestCompareShadowDoesNotExposeAnswers(t *testing.T) {
	item := eval.Case{ID: "case-1", Expected: eval.Expectations{RequiredSubstrings: []string{"verified"}}}
	baseline := eval.Observation{Answer: "legacy private answer", Tools: []string{"search_knowledge"}, Completed: true, Duration: time.Second}
	candidate := eval.Observation{Answer: "verified answer", Tools: []string{"search_knowledge"}, Completed: true, FirstToken: 100 * time.Millisecond, Duration: 800 * time.Millisecond}
	result := CompareShadow(item, baseline, candidate)
	if !result.CandidatePassed || !result.ToolSetsEqual {
		t.Fatalf("unexpected comparison: %#v", result)
	}
	if result.BaselineAnswerSHA256 == "" || result.CandidateAnswerSHA256 == "" {
		t.Fatal("expected answer hashes")
	}
	if result.BaselineAnswerBytes != len([]byte(baseline.Answer)) || result.CandidateAnswerBytes != len([]byte(candidate.Answer)) {
		t.Fatalf("unexpected answer sizes: %#v", result)
	}
}
