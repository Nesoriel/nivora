package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/eval"
)

func main() {
	datasetPath := flag.String("dataset", "evals/support-regression.example.jsonl", "JSONL evaluation dataset")
	baseURL := flag.String("base-url", env("NIVORA_EVAL_BASE_URL", "http://127.0.0.1:3100"), "Nivora base URL")
	sharedSecret := flag.String("key", os.Getenv("NIVORA_EVAL_SHARED_SECRET"), "Nivora internal service key")
	bearerToken := flag.String("bearer", os.Getenv("NIVORA_EVAL_BEARER_TOKEN"), "short-lived Provider context for authenticated cases")
	timeout := flag.Duration("timeout", 120*time.Second, "timeout for each evaluation case")
	outputPath := flag.String("output", "", "optional JSONL result file")
	flag.Parse()

	dataset, err := os.Open(*datasetPath)
	if err != nil {
		fatalf("open dataset: %v", err)
	}
	defer dataset.Close()
	cases, err := eval.LoadJSONL(dataset)
	if err != nil {
		fatalf("load dataset: %v", err)
	}

	var output io.Writer = os.Stdout
	var outputFile *os.File
	if strings.TrimSpace(*outputPath) != "" {
		outputFile, err = os.Create(*outputPath)
		if err != nil {
			fatalf("create output: %v", err)
		}
		defer outputFile.Close()
		output = io.MultiWriter(os.Stdout, outputFile)
	}
	writer := bufio.NewWriter(output)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)

	client := eval.Client{
		BaseURL:      *baseURL,
		SharedSecret: *sharedSecret,
		BearerToken:  *bearerToken,
		HTTPClient:   &http.Client{},
	}

	failed := 0
	for _, item := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		observation, runErr := client.Run(ctx, item)
		cancel()
		var result eval.Result
		if runErr != nil {
			result = eval.Result{
				ID:       item.ID,
				Passed:   false,
				Failures: []string{runErr.Error()},
			}
		} else {
			result = eval.Evaluate(item, observation)
		}
		if !result.Passed {
			failed++
		}
		if err := encoder.Encode(result); err != nil {
			fatalf("write result: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		fatalf("flush results: %v", err)
	}

	fmt.Fprintf(os.Stderr, "evaluated=%d passed=%d failed=%d\n", len(cases), len(cases)-failed, failed)
	if failed > 0 {
		os.Exit(1)
	}
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
