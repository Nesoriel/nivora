package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/knowledgeeval"
	"github.com/Nesoriel/nivora/pkg/knowledge"
)

func main() {
	datasetPath := flag.String("dataset", env("NIVORA_KNOWLEDGE_EVAL_DATASET", "evals/knowledge.example.jsonl"), "JSONL evaluation dataset")
	baseURL := flag.String("url", env("NIVORA_KNOWLEDGE_EVAL_URL", "http://127.0.0.1:3110"), "approved knowledge service URL")
	secret := flag.String("key", strings.TrimSpace(os.Getenv("NIVORA_KNOWLEDGE_SHARED_SECRET")), "knowledge service key")
	outputPath := flag.String("output", "", "optional JSONL output path")
	requestTimeout := flag.Duration("timeout", 20*time.Second, "timeout for each retrieval")
	flag.Parse()

	if strings.TrimSpace(*secret) == "" {
		fatalf("knowledge service key is required")
	}
	file, err := os.Open(*datasetPath)
	if err != nil {
		fatalf("open dataset: %v", err)
	}
	defer file.Close()
	cases, err := knowledgeeval.LoadJSONL(file)
	if err != nil {
		fatalf("load dataset: %v", err)
	}

	writer, closeWriter, err := resultWriter(*outputPath)
	if err != nil {
		fatalf("create result writer: %v", err)
	}
	defer closeWriter()
	encoder := json.NewEncoder(writer)
	client := &http.Client{Timeout: *requestTimeout}
	failed := 0
	for _, item := range cases {
		observation := runCase(context.Background(), client, strings.TrimRight(*baseURL, "/"), *secret, item)
		result := knowledgeeval.Evaluate(item, observation)
		if !result.Passed {
			failed++
		}
		if err := encoder.Encode(result); err != nil {
			fatalf("write result: %v", err)
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d of %d knowledge evaluation cases failed\n", failed, len(cases))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "all %d knowledge evaluation cases passed\n", len(cases))
}

func runCase(parent context.Context, client *http.Client, baseURL, secret string, item knowledgeeval.Case) knowledgeeval.Observation {
	payload, err := json.Marshal(map[string]any{
		"tenant_id": item.TenantID,
		"query":     item.Query,
		"limit":     10,
	})
	if err != nil {
		return knowledgeeval.Observation{Error: err.Error()}
	}
	request, err := http.NewRequestWithContext(parent, http.MethodPost, baseURL+"/v1/search", bytes.NewReader(payload))
	if err != nil {
		return knowledgeeval.Observation{Error: err.Error()}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Nivora-Knowledge-Key", secret)
	started := time.Now()
	response, err := client.Do(request)
	duration := time.Since(started)
	if err != nil {
		return knowledgeeval.Observation{Duration: duration, Error: err.Error()}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8*1024))
		return knowledgeeval.Observation{Duration: duration, Error: fmt.Sprintf("status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))}
	}
	var body struct {
		Items []knowledge.Item `json:"items"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(&body); err != nil {
		return knowledgeeval.Observation{Duration: duration, Error: err.Error()}
	}
	return knowledgeeval.Observation{Items: body.Items, Duration: duration}
}

func resultWriter(path string) (io.Writer, func(), error) {
	if strings.TrimSpace(path) == "" {
		return bufio.NewWriter(os.Stdout), func() {}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() {}, err
	}
	buffer := bufio.NewWriter(file)
	return buffer, func() {
		_ = buffer.Flush()
		_ = file.Close()
	}, nil
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
