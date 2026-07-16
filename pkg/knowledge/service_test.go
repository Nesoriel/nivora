package knowledge

import (
	"context"
	"testing"
	"time"
)

type fakeBackend struct {
	candidates []Candidate
}

func (f fakeBackend) Search(context.Context, Query) ([]Candidate, error) {
	return append([]Candidate(nil), f.candidates...), nil
}

func TestServiceFailsClosedAcrossTenantApprovalAndFreshness(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	expired := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	backend := fakeBackend{candidates: []Candidate{
		validCandidate("tenant-a", "approved", "doc-1", "chunk-1", 0.95, now.Add(-time.Hour), nil),
		validCandidate("tenant-b", "approved", "doc-2", "chunk-2", 0.99, now.Add(-time.Hour), nil),
		validCandidate("tenant-a", "draft", "doc-3", "chunk-3", 0.99, now.Add(-time.Hour), nil),
		validCandidate("tenant-a", "approved", "doc-4", "chunk-4", 0.99, future, nil),
		validCandidate("tenant-a", "approved", "doc-5", "chunk-5", 0.99, now.Add(-time.Hour), &expired),
		validCandidate("tenant-a", "approved", "doc-6", "chunk-6", 0.2, now.Add(-time.Hour), nil),
	}}
	service, err := NewService(backend)
	if err != nil {
		t.Fatal(err)
	}

	items, err := service.Search(context.Background(), Query{
		TenantID: "tenant-a",
		Text:     "refund policy",
		Limit:    10,
		MinScore: 0.8,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ChunkID != "chunk-1" {
		t.Fatalf("unexpected approved results: %#v", items)
	}
}

func TestServiceRequiresCompleteProvenanceAndDeduplicates(t *testing.T) {
	now := time.Now().UTC()
	valid := validCandidate("tenant-a", "approved", "doc-1", "chunk-1", 0.9, now.Add(-time.Hour), nil)
	missingVersion := valid
	missingVersion.ChunkID = "chunk-2"
	missingVersion.SourceVersion = ""
	backend := fakeBackend{candidates: []Candidate{valid, valid, missingVersion}}
	service, _ := NewService(backend)

	items, err := service.Search(context.Background(), Query{TenantID: "tenant-a", Text: "question", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one complete unique result, got %#v", items)
	}
}

func validCandidate(tenant, status, documentID, chunkID string, score float64, effective time.Time, expires *time.Time) Candidate {
	return Candidate{
		TenantID:       tenant,
		DocumentID:     documentID,
		ChunkID:        chunkID,
		SourceTitle:    "Refund policy",
		SourceVersion:  "v3",
		SourceURI:      "https://support.example/policies/refunds",
		Content:        "Verified support content.",
		ApprovalStatus: status,
		EffectiveAt:    effective,
		ExpiresAt:      expires,
		Score:          score,
	}
}
