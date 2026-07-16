package knowledge

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	defaultLimit = 6
	maxLimit     = 20
)

// Service applies the mandatory trust boundary after backend retrieval.
type Service struct {
	backend Backend
	now     func() time.Time
}

// NewService creates an approved-knowledge service.
func NewService(backend Backend) (*Service, error) {
	if backend == nil {
		return nil, errors.New("knowledge backend is required")
	}
	return &Service{backend: backend, now: time.Now}, nil
}

// Search returns only current, approved, same-tenant chunks with complete
// provenance. Invalid backend records are silently dropped rather than exposed.
func (s *Service) Search(ctx context.Context, query Query) ([]Item, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.Text = strings.TrimSpace(query.Text)
	if query.TenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if query.Text == "" {
		return nil, errors.New("query text is required")
	}
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}
	if query.MinScore < 0 {
		query.MinScore = 0
	}
	if query.Now.IsZero() {
		query.Now = s.now().UTC()
	} else {
		query.Now = query.Now.UTC()
	}

	candidates, err := s.backend.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, min(query.Limit, len(candidates)))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate.TenantID = strings.TrimSpace(candidate.TenantID)
		candidate.DocumentID = strings.TrimSpace(candidate.DocumentID)
		candidate.ChunkID = strings.TrimSpace(candidate.ChunkID)
		candidate.SourceTitle = strings.TrimSpace(candidate.SourceTitle)
		candidate.SourceVersion = strings.TrimSpace(candidate.SourceVersion)
		candidate.SourceURI = strings.TrimSpace(candidate.SourceURI)
		candidate.Content = strings.TrimSpace(candidate.Content)
		candidate.ApprovalStatus = strings.ToLower(strings.TrimSpace(candidate.ApprovalStatus))

		if candidate.TenantID != query.TenantID || candidate.ApprovalStatus != "approved" {
			continue
		}
		if candidate.DocumentID == "" || candidate.ChunkID == "" || candidate.SourceTitle == "" || candidate.SourceVersion == "" || candidate.Content == "" {
			continue
		}
		if candidate.Score < query.MinScore {
			continue
		}
		if !candidate.EffectiveAt.IsZero() && candidate.EffectiveAt.After(query.Now) {
			continue
		}
		if candidate.ExpiresAt != nil && !candidate.ExpiresAt.After(query.Now) {
			continue
		}
		key := candidate.DocumentID + "\x00" + candidate.ChunkID + "\x00" + candidate.SourceVersion
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, Item{
			DocumentID:  candidate.DocumentID,
			ChunkID:     candidate.ChunkID,
			Title:       candidate.SourceTitle,
			Version:     candidate.SourceVersion,
			Source:      candidate.SourceURI,
			Content:     candidate.Content,
			Score:       candidate.Score,
			EffectiveAt: candidate.EffectiveAt.UTC(),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].ChunkID < items[j].ChunkID
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
