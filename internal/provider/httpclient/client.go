package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

const maxResponseBytes = 2 << 20

// Option configures a Provider HTTP client.
type Option func(*Client)

// WithRetry enables bounded retries for idempotent Provider requests.
func WithRetry(maxRetries int, backoff time.Duration) Option {
	return func(client *Client) {
		client.maxRetries = maxRetries
		client.retryBackoff = backoff
	}
}

// ResponseError is a sanitized Provider HTTP failure.
type ResponseError struct {
	StatusCode int
	Endpoint   string
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("provider returned status %d for %s", e.StatusCode, e.Endpoint)
}

// Client implements provider.Provider over a versioned HTTP API.
type Client struct {
	baseURL      *url.URL
	sharedSecret string
	httpClient   *http.Client
	maxRetries   int
	retryBackoff time.Duration
}

// New creates a provider HTTP client.
func New(baseURL, sharedSecret string, httpClient *http.Client, options ...Option) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse provider base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("provider base URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("provider base URL must include a host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := &Client{baseURL: parsed, sharedSecret: sharedSecret, httpClient: httpClient}
	for _, option := range options {
		option(client)
	}
	return client, nil
}

// Check verifies that the Provider is reachable and speaks a supported v1 contract.
func (c *Client) Check(ctx context.Context) error {
	capabilities, err := c.Capabilities(ctx, provider.RequestAuth{})
	if err != nil {
		return err
	}
	if strings.TrimSpace(capabilities.Provider) == "" {
		return errors.New("provider capabilities response is missing provider")
	}
	if !supportsV1(capabilities.Version) {
		return fmt.Errorf("unsupported provider API version %q", capabilities.Version)
	}
	return nil
}

func (c *Client) Capabilities(ctx context.Context, auth provider.RequestAuth) (domain.CapabilitySet, error) {
	var out domain.CapabilitySet
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/capabilities", nil, nil, nil, &out)
	return out, err
}

func (c *Client) CustomerContext(ctx context.Context, auth provider.RequestAuth) (domain.CustomerContext, error) {
	var out domain.CustomerContext
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/context", nil, nil, nil, &out)
	return out, err
}

func (c *Client) SearchKnowledge(ctx context.Context, auth provider.RequestAuth, query string, limit int) ([]domain.KnowledgeItem, error) {
	values := url.Values{"q": []string{query}, "limit": []string{strconv.Itoa(limit)}}
	var out struct {
		Items []domain.KnowledgeItem `json:"items"`
	}
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/knowledge", values, nil, nil, &out)
	return out.Items, err
}

func (c *Client) ListResources(ctx context.Context, auth provider.RequestAuth, limit int, status string) ([]domain.Resource, error) {
	values := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if status != "" {
		values.Set("status", status)
	}
	var out struct {
		Items []domain.Resource `json:"items"`
	}
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/resources", values, nil, nil, &out)
	return out.Items, err
}

func (c *Client) DiagnoseResource(ctx context.Context, auth provider.RequestAuth, resourceID string) (domain.Diagnosis, error) {
	values := url.Values{"resource_id": []string{resourceID}}
	var out domain.Diagnosis
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/diagnosis", values, nil, nil, &out)
	return out, err
}

func (c *Client) ListTransactions(ctx context.Context, auth provider.RequestAuth, resourceID string, limit int) ([]domain.Transaction, error) {
	values := url.Values{"resource_id": []string{resourceID}, "limit": []string{strconv.Itoa(limit)}}
	var out struct {
		Items []domain.Transaction `json:"items"`
	}
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/transactions", values, nil, nil, &out)
	return out.Items, err
}

func (c *Client) CreateCase(ctx context.Context, auth provider.RequestAuth, input domain.CreateCaseInput) (domain.SupportCase, error) {
	var out domain.SupportCase
	headers := make(http.Header)
	if input.IdempotencyKey != "" {
		headers.Set("Idempotency-Key", input.IdempotencyKey)
	}
	err := c.doJSON(ctx, auth, http.MethodPost, "/api/internal/support/cases", nil, headers, input, &out)
	return out, err
}

func (c *Client) doJSON(ctx context.Context, auth provider.RequestAuth, method, endpoint string, query url.Values, headers http.Header, body any, out any) error {
	target := *c.baseURL
	target.Path = path.Join(target.Path, endpoint)
	target.RawQuery = query.Encode()

	var bodyBytes []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode provider request: %w", err)
		}
		bodyBytes = encoded
	}

	attempts := 1
	if idempotent(method) {
		attempts += c.maxRetries
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := waitRetry(ctx, c.retryBackoff, attempt-1); err != nil {
				return err
			}
		}

		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
		if err != nil {
			return fmt.Errorf("create provider request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for name, values := range headers {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
		if auth.BearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
		}
		if c.sharedSecret != "" {
			req.Header.Set("X-Nivora-Provider-Key", c.sharedSecret)
		}

		response, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("provider request failed: %w", err)
			if attempt+1 < attempts && ctx.Err() == nil {
				continue
			}
			return lastErr
		}

		raw, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read provider response: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close provider response: %w", closeErr)
		}
		if len(raw) > maxResponseBytes {
			return errors.New("provider response exceeded size limit")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			lastErr = &ResponseError{StatusCode: response.StatusCode, Endpoint: endpoint}
			if attempt+1 < attempts && retryableStatus(response.StatusCode) {
				continue
			}
			return lastErr
		}
		if out == nil || len(raw) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode provider response: %w", err)
		}
		return nil
	}
	return lastErr
}

func supportsV1(version string) bool {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	return version == "1" || strings.HasPrefix(version, "1.")
}

func idempotent(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func waitRetry(ctx context.Context, base time.Duration, attempt int) error {
	if base <= 0 {
		return nil
	}
	delay := base
	for index := 0; index < attempt && delay < 2*time.Second; index++ {
		delay *= 2
	}
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
