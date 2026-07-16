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

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

const maxResponseBytes = 2 << 20

// Client implements provider.Provider over a versioned HTTP API.
type Client struct {
	baseURL      *url.URL
	sharedSecret string
	httpClient   *http.Client
}

// New creates a provider HTTP client.
func New(baseURL, sharedSecret string, httpClient *http.Client) (*Client, error) {
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
	return &Client{baseURL: parsed, sharedSecret: sharedSecret, httpClient: httpClient}, nil
}

func (c *Client) Capabilities(ctx context.Context, auth provider.RequestAuth) (domain.CapabilitySet, error) {
	var out domain.CapabilitySet
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/capabilities", nil, nil, &out)
	return out, err
}

func (c *Client) CustomerContext(ctx context.Context, auth provider.RequestAuth) (domain.CustomerContext, error) {
	var out domain.CustomerContext
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/context", nil, nil, &out)
	return out, err
}

func (c *Client) SearchKnowledge(ctx context.Context, auth provider.RequestAuth, query string, limit int) ([]domain.KnowledgeItem, error) {
	values := url.Values{"q": []string{query}, "limit": []string{strconv.Itoa(limit)}}
	var out struct {
		Items []domain.KnowledgeItem `json:"items"`
	}
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/knowledge", values, nil, &out)
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
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/resources", values, nil, &out)
	return out.Items, err
}

func (c *Client) DiagnoseResource(ctx context.Context, auth provider.RequestAuth, resourceID string) (domain.Diagnosis, error) {
	values := url.Values{"resource_id": []string{resourceID}}
	var out domain.Diagnosis
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/diagnosis", values, nil, &out)
	return out, err
}

func (c *Client) ListTransactions(ctx context.Context, auth provider.RequestAuth, resourceID string, limit int) ([]domain.Transaction, error) {
	values := url.Values{"resource_id": []string{resourceID}, "limit": []string{strconv.Itoa(limit)}}
	var out struct {
		Items []domain.Transaction `json:"items"`
	}
	err := c.doJSON(ctx, auth, http.MethodGet, "/api/internal/support/transactions", values, nil, &out)
	return out.Items, err
}

func (c *Client) CreateCase(ctx context.Context, auth provider.RequestAuth, input domain.CreateCaseInput) (domain.SupportCase, error) {
	var out domain.SupportCase
	err := c.doJSON(ctx, auth, http.MethodPost, "/api/internal/support/cases", nil, input, &out)
	return out, err
}

func (c *Client) doJSON(ctx context.Context, auth provider.RequestAuth, method, endpoint string, query url.Values, body any, out any) error {
	target := *c.baseURL
	target.Path = path.Join(target.Path, endpoint)
	target.RawQuery = query.Encode()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode provider request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return fmt.Errorf("create provider request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	}
	if c.sharedSecret != "" {
		req.Header.Set("X-Nivora-Provider-Key", c.sharedSecret)
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("provider request failed: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read provider response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return errors.New("provider response exceeded size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode provider response: %w", err)
	}
	return nil
}
