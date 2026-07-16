package eval

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
)

const maxEvaluationResponseBytes = 4 << 20

// Client runs synthetic cases against a deployed Nivora instance.
type Client struct {
	BaseURL      string
	SharedSecret string
	BearerToken  string
	HTTPClient   *http.Client
}

// Run executes one case and collects only the public SSE behavior.
func (c Client) Run(ctx context.Context, item Case) (Observation, error) {
	if strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.SharedSecret) == "" {
		return Observation{}, fmt.Errorf("evaluation base URL and shared secret are required")
	}
	if item.Principal.Authenticated && strings.TrimSpace(c.BearerToken) == "" {
		return Observation{}, fmt.Errorf("authenticated evaluation case %q requires a bearer token", item.ID)
	}

	payload, err := json.Marshal(domain.ChatRequest{
		Question:  item.Question,
		History:   item.History,
		Tenant:    item.Tenant,
		Principal: item.Principal,
	})
	if err != nil {
		return Observation{}, fmt.Errorf("encode evaluation request: %w", err)
	}

	target := strings.TrimRight(c.BaseURL, "/") + "/v1/chat/stream"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return Observation{}, fmt.Errorf("create evaluation request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("X-Nivora-Key", c.SharedSecret)
	if item.Principal.Authenticated {
		request.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	started := time.Now()
	response, err := httpClient.Do(request)
	if err != nil {
		return Observation{}, fmt.Errorf("run evaluation request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return Observation{}, fmt.Errorf("evaluation request returned %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	observation := Observation{}
	limited := io.LimitReader(response.Body, maxEvaluationResponseBytes)
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event domain.StreamEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return Observation{}, fmt.Errorf("decode evaluation SSE event: %w", err)
		}
		switch event.Type {
		case "message.delta":
			observation.Answer += event.Content
		case "tool.started":
			observation.Tools = append(observation.Tools, event.ToolName)
		case "done":
			observation.Completed = true
		case "error":
			observation.ErrorCode = event.Code
			if event.Content != "" {
				observation.Answer += event.Content
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Observation{}, fmt.Errorf("read evaluation SSE stream: %w", err)
	}
	observation.Duration = time.Since(started)
	return observation, nil
}
