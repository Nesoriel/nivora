package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Nesoriel/nivora/internal/acceptance"
	"github.com/Nesoriel/nivora/internal/eval"
)

func main() {
	datasetPath := flag.String("dataset", "evals/support-regression.example.jsonl", "JSONL dataset; the first case is used as the load template")
	baseURL := flag.String("base-url", env("NIVORA_LOAD_BASE_URL", "http://127.0.0.1:3100"), "Nivora base URL")
	sharedSecret := flag.String("key", os.Getenv("NIVORA_LOAD_SHARED_SECRET"), "Nivora internal service key")
	bearerToken := flag.String("bearer", os.Getenv("NIVORA_LOAD_BEARER_TOKEN"), "short-lived Provider context")
	requests := flag.Int("requests", 20, "total request count")
	concurrency := flag.Int("concurrency", 4, "maximum concurrent requests")
	timeout := flag.Duration("timeout", 120*time.Second, "timeout per request")
	minimumSuccess := flag.Float64("minimum-success-rate", 0.99, "required success ratio from 0 to 1")
	maximumP95 := flag.Duration("maximum-p95", 0, "optional maximum p95 completion latency")
	flag.Parse()

	if *requests < 1 || *concurrency < 1 || *concurrency > *requests {
		fatalf("requests and concurrency must be positive and concurrency must not exceed requests")
	}
	if *minimumSuccess < 0 || *minimumSuccess > 1 {
		fatalf("minimum-success-rate must be between 0 and 1")
	}
	file, err := os.Open(*datasetPath)
	if err != nil {
		fatalf("open dataset: %v", err)
	}
	defer file.Close()
	cases, err := eval.LoadJSONL(file)
	if err != nil {
		fatalf("load dataset: %v", err)
	}
	template := cases[0]
	client := eval.Client{BaseURL: *baseURL, SharedSecret: *sharedSecret, BearerToken: *bearerToken, HTTPClient: &http.Client{}}

	jobs := make(chan int)
	results := make(chan acceptance.LoadSample, *requests)
	var workers sync.WaitGroup
	for worker := 0; worker < *concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				observation, runErr := client.Run(ctx, template)
				cancel()
				sample := acceptance.LoadSample{
					FirstToken: observation.FirstToken,
					Completion: observation.Duration,
					Success:    runErr == nil && observation.Completed && observation.ErrorCode == "",
					ErrorCode:  observation.ErrorCode,
				}
				if runErr != nil {
					sample.ErrorCode = classifyError(runErr.Error())
				}
				if !observation.Completed && sample.ErrorCode == "" {
					sample.ErrorCode = "stream_incomplete"
				}
				results <- sample
			}
		}()
	}
	go func() {
		for index := 0; index < *requests; index++ {
			jobs <- index
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	samples := make([]acceptance.LoadSample, 0, *requests)
	for sample := range results {
		samples = append(samples, sample)
	}
	summary := acceptance.SummarizeLoad(samples)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fatalf("write load summary: %v", err)
	}
	failed := summary.SuccessRate < *minimumSuccess
	if *maximumP95 > 0 && time.Duration(summary.CompletionP95MS)*time.Millisecond > *maximumP95 {
		failed = true
	}
	if failed {
		os.Exit(1)
	}
}

func classifyError(message string) string {
	message = strings.ToLower(message)
	for _, code := range []string{"service_busy", "provider_context_required", "dependency_unavailable", "runtime_not_configured", "agent_run_failed"} {
		if strings.Contains(message, code) {
			return code
		}
	}
	if strings.Contains(message, "context deadline") || strings.Contains(message, "timeout") {
		return "timeout"
	}
	return "request_error"
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func fatalf(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(2)
}
