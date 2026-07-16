package knowledge

import (
	"context"
	"time"
)

// Query describes one tenant-scoped approved-knowledge lookup.
type Query struct {
	TenantID string
	Text     string
	Limit    int
	MinScore float64
	Now      time.Time
}

// Candidate is a backend result before the approval service applies its
// fail-closed validation. Backends must populate every provenance field.
type Candidate struct {
	TenantID       string
	DocumentID     string
	ChunkID        string
	SourceTitle    string
	SourceVersion  string
	SourceURI      string
	Content        string
	ApprovalStatus string
	EffectiveAt    time.Time
	ExpiresAt      *time.Time
	Score          float64
}

// Item is safe to expose through a product Provider API.
type Item struct {
	DocumentID  string    `json:"document_id"`
	ChunkID     string    `json:"chunk_id"`
	Title       string    `json:"title"`
	Version     string    `json:"version"`
	Source      string    `json:"source,omitempty"`
	Content     string    `json:"content"`
	Score       float64   `json:"score"`
	EffectiveAt time.Time `json:"effective_at"`
}

// Backend performs semantic retrieval. Tenant/approval filters must be pushed
// down where supported, but Service validates them again before returning data.
type Backend interface {
	Search(context.Context, Query) ([]Candidate, error)
}
