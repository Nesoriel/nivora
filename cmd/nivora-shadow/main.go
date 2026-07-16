package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/acceptance"
	"github.com/Nesoriel/nivora/internal/eval"
)

func main() {
	datasetPath := flag.String("dataset", "evals/support-regression.example.jsonl", "synthetic or approved-redacted JSONL dataset")
	baselineURL := flag.String("baseline-url", env("NIVORA_SHADOW_BASELINE_URL", ""), "baseline Nivora-compatible endpoint")
	candidateURL := flag.String("candidate-url", env("NIVORA_SHADOW_CANDIDATE_URL", "http://127.0.0.1:3100"), "candidate Nivora endpoint")
	baselineKey := flag.String("baseline-key", os.Getenv("NIVORA_SHADOW_BASELINE_KEY"), "baseline internal service key")
	candidateKey := flag.String("candidate-key", os.Getenv("NIVORA_SHADOW_CANDIDATE_KEY"), "candidate internal service key")
	baselineBearer := flag.String("baseline-bearer", os.Getenv("NIVORA_SHADOW_BASELINE_BEARER"), "baseline short-lived Provider context")
	candidateBearer := flag.String("candidate-bearer", os.Getenv("NIVORA_SHADOW_CANDIDATE_BEARER"), "candidate short-lived Provider context")
	timeout := flag.Duration("timeout", 120*time.Second, "timeout per endpoint and case")
	outputPath := flag.String("output", "", "optional JSONL output path")
	flag.Parse()

	if strings.TrimSpace(*baselineURL) == "" || strings.TrimSpace(*candidateURL) == "" {
		fatalf("baseline-url and candidate-url are required")
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

	var output = os.Stdout
	if strings.TrimSpace(*outputPath) != "" {
		output, err = os.Create(*outputPath)
		if err != nil {
			fatalf("create output: %v", err)
		}
		defer output.Close()
	}
	writer := bufio.NewWriter(output)
	encoder := json.NewEncoder(writer)
	baselineClient := eval.Client{BaseURL: *baselineURL, SharedSecret: *baselineKey, BearerToken: *baselineBearer, HTTPClient: &http.Client{}}
	candidateClient := eval.Client{BaseURL: *candidateURL, SharedSecret: *candidateKey, BearerToken: *candidateBearer, HTTPClient: &http.Client{}}
	failed := 0
	for _, item := range cases {
		baseline, baselineErr := run(context.Background(), baselineClient, item, *timeout)
		candidate, candidateErr := run(context.Background(), candidateClient, item, *timeout)
		result := acceptance.CompareShadow(item, baseline, candidate)
		if baselineErr != nil {
			result.BaselineErrorCode = "request_error"
		}
		if candidateErr != nil {
			result.CandidatePassed = false
			result.CandidateErrorCode = "request_error"
			result.CandidateFailures = append(result.CandidateFailures, "candidate request failed")
		}
		if !result.CandidatePassed {
			failed++
		}
		if err := encoder.Encode(result); err != nil {
			fatalf("write shadow result: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		fatalf("flush shadow results: %v", err)
	}
	fmt.Fprintf(os.Stderr, "shadow_cases=%d candidate_passed=%d candidate_failed=%d\n", len(cases), len(cases)-failed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func run(parent context.Context, client eval.Client, item eval.Case, timeout time.Duration) (eval.Observation, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return client.Run(ctx, item)
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
