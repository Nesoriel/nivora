package acceptance

import (
	"testing"
	"time"
)

func TestSummarizeLoad(t *testing.T) {
	summary := SummarizeLoad([]LoadSample{
		{Success: true, FirstToken: 10 * time.Millisecond, Completion: 100 * time.Millisecond},
		{Success: true, FirstToken: 20 * time.Millisecond, Completion: 200 * time.Millisecond},
		{Success: true, FirstToken: 30 * time.Millisecond, Completion: 300 * time.Millisecond},
		{Success: false, ErrorCode: "service_busy"},
	})
	if summary.Requests != 4 || summary.Successful != 3 || summary.Failed != 1 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	if summary.FirstTokenP50MS != 20 || summary.FirstTokenP95MS != 30 || summary.CompletionP99MS != 300 {
		t.Fatalf("unexpected percentiles: %#v", summary)
	}
	if summary.Errors["service_busy"] != 1 {
		t.Fatalf("unexpected errors: %#v", summary.Errors)
	}
}
