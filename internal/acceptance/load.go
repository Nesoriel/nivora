package acceptance

import (
	"sort"
	"time"
)

// LoadSample is one completed load-test request.
type LoadSample struct {
	FirstToken time.Duration
	Completion time.Duration
	Success    bool
	ErrorCode  string
}

// LoadSummary is the stable machine-readable load result.
type LoadSummary struct {
	Requests        int            `json:"requests"`
	Successful      int            `json:"successful"`
	Failed          int            `json:"failed"`
	SuccessRate     float64        `json:"success_rate"`
	FirstTokenP50MS int64          `json:"first_token_p50_ms"`
	FirstTokenP95MS int64          `json:"first_token_p95_ms"`
	FirstTokenP99MS int64          `json:"first_token_p99_ms"`
	CompletionP50MS int64          `json:"completion_p50_ms"`
	CompletionP95MS int64          `json:"completion_p95_ms"`
	CompletionP99MS int64          `json:"completion_p99_ms"`
	Errors          map[string]int `json:"errors,omitempty"`
}

// SummarizeLoad calculates stable nearest-rank percentiles.
func SummarizeLoad(samples []LoadSample) LoadSummary {
	summary := LoadSummary{Requests: len(samples), Errors: make(map[string]int)}
	firstTokens := make([]time.Duration, 0, len(samples))
	completions := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		if sample.Success {
			summary.Successful++
			if sample.FirstToken > 0 {
				firstTokens = append(firstTokens, sample.FirstToken)
			}
			if sample.Completion > 0 {
				completions = append(completions, sample.Completion)
			}
		} else {
			summary.Failed++
			code := sample.ErrorCode
			if code == "" {
				code = "unknown"
			}
			summary.Errors[code]++
		}
	}
	if summary.Requests > 0 {
		summary.SuccessRate = float64(summary.Successful) / float64(summary.Requests)
	}
	summary.FirstTokenP50MS = percentile(firstTokens, 0.50).Milliseconds()
	summary.FirstTokenP95MS = percentile(firstTokens, 0.95).Milliseconds()
	summary.FirstTokenP99MS = percentile(firstTokens, 0.99).Milliseconds()
	summary.CompletionP50MS = percentile(completions, 0.50).Milliseconds()
	summary.CompletionP95MS = percentile(completions, 0.95).Milliseconds()
	summary.CompletionP99MS = percentile(completions, 0.99).Milliseconds()
	if len(summary.Errors) == 0 {
		summary.Errors = nil
	}
	return summary
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	index := int(float64(len(copyValues))*quantile+0.999999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copyValues) {
		index = len(copyValues) - 1
	}
	return copyValues[index]
}
