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

	"github.com/Nesoriel/nivora/internal/acceptance"
)

func main() {
	datasetPath := flag.String("dataset", "evals/security-probes.example.jsonl", "JSONL security probe dataset")
	baseURL := flag.String("base-url", env("NIVORA_PROBE_BASE_URL", "http://127.0.0.1:3100"), "Nivora base URL")
	sharedSecret := flag.String("key", os.Getenv("NIVORA_PROBE_SHARED_SECRET"), "Nivora internal service key")
	bearerToken := flag.String("bearer", os.Getenv("NIVORA_PROBE_BEARER_TOKEN"), "valid short-lived Provider context")
	timeout := flag.Duration("timeout", 10*time.Second, "timeout per probe")
	outputPath := flag.String("output", "", "optional JSONL output path")
	flag.Parse()

	file, err := os.Open(*datasetPath)
	if err != nil {
		fatalf("open probe dataset: %v", err)
	}
	defer file.Close()
	cases, err := acceptance.LoadProbeJSONL(file)
	if err != nil {
		fatalf("load probe dataset: %v", err)
	}
	client := acceptance.ProbeClient{
		BaseURL:      *baseURL,
		SharedSecret: *sharedSecret,
		BearerToken:  *bearerToken,
		HTTPClient:   &http.Client{},
	}
	var output io.Writer = os.Stdout
	var outputFile *os.File
	if strings.TrimSpace(*outputPath) != "" {
		outputFile, err = os.Create(*outputPath)
		if err != nil {
			fatalf("create probe output: %v", err)
		}
		defer outputFile.Close()
		output = io.MultiWriter(os.Stdout, outputFile)
	}
	writer := bufio.NewWriter(output)
	encoder := json.NewEncoder(writer)
	failed := 0
	for _, item := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		result, runErr := client.Run(ctx, item)
		cancel()
		if runErr != nil {
			result = acceptance.ProbeResult{ID: item.ID, Passed: false, Failures: []string{runErr.Error()}}
		}
		if !result.Passed {
			failed++
		}
		if err := encoder.Encode(result); err != nil {
			fatalf("write probe result: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		fatalf("flush probe results: %v", err)
	}
	fmt.Fprintf(os.Stderr, "probes=%d passed=%d failed=%d\n", len(cases), len(cases)-failed, failed)
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
