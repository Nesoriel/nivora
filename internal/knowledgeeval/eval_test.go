package knowledgeeval

import (
	"testing"
	"time"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

func TestEvaluateDetectsMissingAndForbiddenDocuments(t *testing.T) {
	result := Evaluate(Case{
		ID:                   "refund",
		ExpectedDocumentIDs:  []string{"refund-policy"},
		ForbiddenDocumentIDs: []string{"internal-recipe"},
		MinimumResults:       1,
		MaximumLatencyMS:     100,
	}, Observation{
		Items: []knowledge.Item{{DocumentID: "internal-recipe"}},
		Duration: 150 * time.Millisecond,
	})
	if result.Passed {
		t.Fatal("expected evaluation failure")
	}
	if len(result.Failures) != 3 {
		t.Fatalf("unexpected failures: %#v", result.Failures)
	}
}

func TestEvaluatePassesExpectedSource(t *testing.T) {
	result := Evaluate(Case{
		ID:                  "help",
		ExpectedDocumentIDs: []string{"help-guide"},
		MinimumResults:      1,
	}, Observation{Items: []knowledge.Item{{DocumentID: "help-guide"}}})
	if !result.Passed {
		t.Fatalf("unexpected failure: %#v", result.Failures)
	}
}
