package vikingdb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	volcretriever "github.com/cloudwego/eino-ext/components/retriever/volc_vikingdb"
	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

const defaultOversample = 3

// FieldMapping maps the approved knowledge schema to VikingDB scalar fields.
type FieldMapping struct {
	TenantID       string
	DocumentID     string
	ChunkID        string
	SourceTitle    string
	SourceVersion  string
	SourceURI      string
	ApprovalStatus string
	EffectiveAt    string
	ExpiresAt      string
}

// DefaultFieldMapping returns the recommended collection schema.
func DefaultFieldMapping() FieldMapping {
	return FieldMapping{
		TenantID:       "tenant_id",
		DocumentID:     "document_id",
		ChunkID:        "chunk_id",
		SourceTitle:    "source_title",
		SourceVersion:  "source_version",
		SourceURI:      "source_uri",
		ApprovalStatus: "approval_status",
		EffectiveAt:    "effective_at",
		ExpiresAt:      "expires_at",
	}
}

// Config configures the Provider-side VikingDB reference adapter.
type Config struct {
	Host              string
	Region            string
	AK                string
	SK                string
	Scheme            string
	Collection        string
	Index             string
	Partition         string
	ConnectionTimeout int64
	WithMultiModal    bool
	EmbeddingModel    string
	UseSparse         bool
	DenseWeight       float64
	Oversample        int
	Fields            FieldMapping
}

// DocumentRetriever is the official Eino Retriever surface used by the adapter.
type DocumentRetriever interface {
	Retrieve(context.Context, string, ...einoretriever.Option) ([]*schema.Document, error)
}

// Backend implements knowledge.Backend using the official Eino VikingDB component.
type Backend struct {
	retriever  DocumentRetriever
	fields     FieldMapping
	oversample int
}

// New creates a VikingDB backend. SDK panics during its eager Ping are converted
// to errors so an invalid configuration cannot crash a Provider process.
func New(ctx context.Context, config Config) (backend *Backend, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			backend = nil
			err = fmt.Errorf("initialize VikingDB retriever: %v", recovered)
		}
	}()

	config.Host = strings.TrimSpace(config.Host)
	config.Region = strings.TrimSpace(config.Region)
	config.Collection = strings.TrimSpace(config.Collection)
	config.Index = strings.TrimSpace(config.Index)
	if config.Host == "" || config.Region == "" || config.Collection == "" || config.Index == "" {
		return nil, errors.New("VikingDB host, region, collection, and index are required")
	}
	if config.Scheme == "" {
		config.Scheme = "https"
	}
	if !config.WithMultiModal && strings.TrimSpace(config.EmbeddingModel) == "" {
		return nil, errors.New("VikingDB embedding model is required when platform vectorization is disabled")
	}
	if config.Oversample <= 0 {
		config.Oversample = defaultOversample
	}
	config.Fields = normalizeFields(config.Fields)

	topK := 100
	retriever, err := volcretriever.NewRetriever(ctx, &volcretriever.RetrieverConfig{
		Host:              config.Host,
		Region:            config.Region,
		AK:                config.AK,
		SK:                config.SK,
		Scheme:            config.Scheme,
		ConnectionTimeout: config.ConnectionTimeout,
		Collection:        config.Collection,
		Index:             config.Index,
		Partition:         config.Partition,
		TopK:              &topK,
		WithMultiModal:    config.WithMultiModal,
		EmbeddingConfig: volcretriever.EmbeddingConfig{
			UseBuiltin:  !config.WithMultiModal,
			ModelName:   config.EmbeddingModel,
			UseSparse:   config.UseSparse,
			DenseWeight: config.DenseWeight,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create VikingDB retriever: %w", err)
	}
	return &Backend{retriever: retriever, fields: config.Fields, oversample: config.Oversample}, nil
}

// NewWithRetriever creates a backend around a supplied Retriever for tests or
// Provider-specific construction.
func NewWithRetriever(retriever DocumentRetriever, fields FieldMapping, oversample int) (*Backend, error) {
	if retriever == nil {
		return nil, errors.New("document retriever is required")
	}
	if oversample <= 0 {
		oversample = defaultOversample
	}
	return &Backend{retriever: retriever, fields: normalizeFields(fields), oversample: oversample}, nil
}

// Search pushes tenant and approval filters into VikingDB, then maps complete
// provenance for the outer knowledge.Service to validate again.
func (b *Backend) Search(ctx context.Context, query knowledge.Query) ([]knowledge.Candidate, error) {
	limit := query.Limit * b.oversample
	if limit < query.Limit {
		limit = query.Limit
	}
	if limit > 100 {
		limit = 100
	}
	filter := map[string]any{
		"op": "and",
		"conditions": []map[string]any{
			{"op": "term", "field": b.fields.TenantID, "value": query.TenantID},
			{"op": "term", "field": b.fields.ApprovalStatus, "value": "approved"},
		},
	}
	documents, err := b.retriever.Retrieve(
		ctx,
		query.Text,
		einoretriever.WithTopK(limit),
		einoretriever.WithScoreThreshold(query.MinScore),
		einoretriever.WithDSLInfo(filter),
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve approved VikingDB knowledge: %w", err)
	}

	candidates := make([]knowledge.Candidate, 0, len(documents))
	for _, document := range documents {
		if document == nil {
			continue
		}
		fields, _ := document.MetaData[volcretriever.ExtraKeyVikingDBFields].(map[string]any)
		candidate := knowledge.Candidate{
			TenantID:       stringField(fields, b.fields.TenantID),
			DocumentID:     firstNonEmpty(stringField(fields, b.fields.DocumentID), document.ID),
			ChunkID:        firstNonEmpty(stringField(fields, b.fields.ChunkID), document.ID),
			SourceTitle:    stringField(fields, b.fields.SourceTitle),
			SourceVersion:  stringField(fields, b.fields.SourceVersion),
			SourceURI:      stringField(fields, b.fields.SourceURI),
			Content:        document.Content,
			ApprovalStatus: stringField(fields, b.fields.ApprovalStatus),
			EffectiveAt:    timeField(fields, b.fields.EffectiveAt),
			ExpiresAt:      optionalTimeField(fields, b.fields.ExpiresAt),
			Score:          document.Score(),
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func normalizeFields(fields FieldMapping) FieldMapping {
	defaults := DefaultFieldMapping()
	if strings.TrimSpace(fields.TenantID) == "" {
		fields.TenantID = defaults.TenantID
	}
	if strings.TrimSpace(fields.DocumentID) == "" {
		fields.DocumentID = defaults.DocumentID
	}
	if strings.TrimSpace(fields.ChunkID) == "" {
		fields.ChunkID = defaults.ChunkID
	}
	if strings.TrimSpace(fields.SourceTitle) == "" {
		fields.SourceTitle = defaults.SourceTitle
	}
	if strings.TrimSpace(fields.SourceVersion) == "" {
		fields.SourceVersion = defaults.SourceVersion
	}
	if strings.TrimSpace(fields.SourceURI) == "" {
		fields.SourceURI = defaults.SourceURI
	}
	if strings.TrimSpace(fields.ApprovalStatus) == "" {
		fields.ApprovalStatus = defaults.ApprovalStatus
	}
	if strings.TrimSpace(fields.EffectiveAt) == "" {
		fields.EffectiveAt = defaults.EffectiveAt
	}
	if strings.TrimSpace(fields.ExpiresAt) == "" {
		fields.ExpiresAt = defaults.ExpiresAt
	}
	return fields
}

func stringField(fields map[string]any, name string) string {
	value, ok := fields[name]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func timeField(fields map[string]any, name string) time.Time {
	parsed, _ := parseTime(fields[name])
	return parsed
}

func optionalTimeField(fields map[string]any, name string) *time.Time {
	parsed, ok := parseTime(fields[name])
	if !ok {
		return nil
	}
	return &parsed
}

func parseTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC(), true
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return time.Time{}, false
		}
		if parsed, err := time.Parse(time.RFC3339, typed); err == nil {
			return parsed.UTC(), true
		}
		if unix, err := strconv.ParseInt(typed, 10, 64); err == nil {
			return time.Unix(unix, 0).UTC(), true
		}
	case int64:
		return time.Unix(typed, 0).UTC(), true
	case int:
		return time.Unix(int64(typed), 0).UTC(), true
	case float64:
		return time.Unix(int64(typed), 0).UTC(), true
	}
	return time.Time{}, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
