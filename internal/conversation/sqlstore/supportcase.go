package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nesoriel/nivora/internal/conversation"
)

// RecordSupportCase upserts the external case reference without persisting the
// original Tool arguments, Provider response, or customer identity.
func (s *Store) RecordSupportCase(ctx context.Context, record conversation.SupportCaseRecord) error {
	record.TenantID = strings.TrimSpace(record.TenantID)
	record.ConversationID = strings.TrimSpace(record.ConversationID)
	record.ProviderCaseID = strings.TrimSpace(record.ProviderCaseID)
	record.Status = strings.TrimSpace(record.Status)
	if record.TenantID == "" || record.ConversationID == "" || record.ProviderCaseID == "" {
		return errors.New("support case identity is required")
	}
	createdMS := millis(record.CreatedAt)
	updatedMS := millis(record.UpdatedAt)
	_, err := s.db.ExecContext(ctx, s.bind(`
		INSERT INTO support_case_refs (
			tenant_id, provider_case_id, conversation_id, status, created_at_ms, updated_at_ms
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, provider_case_id)
		DO UPDATE SET conversation_id = excluded.conversation_id,
			status = excluded.status,
			updated_at_ms = excluded.updated_at_ms
	`), record.TenantID, record.ProviderCaseID, record.ConversationID, record.Status, createdMS, updatedMS)
	if err != nil {
		return fmt.Errorf("record support case: %w", err)
	}
	return nil
}
