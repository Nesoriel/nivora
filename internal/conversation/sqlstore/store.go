package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/Nesoriel/nivora/internal/conversation"
)

// Store implements durable conversation storage over SQLite or PostgreSQL.
type Store struct {
	db      *sql.DB
	dialect string
}

// Open opens, configures, and migrates a durable store.
func Open(ctx context.Context, driver, dsn string) (*Store, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("storage DSN is required")
	}
	sqlDriver := driver
	switch driver {
	case "sqlite":
		sqlDriver = "sqlite"
	case "postgres", "postgresql", "pgx":
		driver = "pgx"
		sqlDriver = "pgx"
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", driver)
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open conversation database: %w", err)
	}
	store := &Store{db: db, dialect: driver}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(checkCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping conversation database: %w", err)
	}
	if err := store.migrate(checkCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) BeginRun(ctx context.Context, record conversation.RunRecord) error {
	if err := validateRun(record); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	nowMS := millis(record.StartedAt)
	if _, err := tx.ExecContext(ctx, s.bind(`
		INSERT INTO conversations (tenant_id, conversation_id, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (tenant_id, conversation_id)
		DO UPDATE SET updated_at_ms = excluded.updated_at_ms
	`), record.TenantID, record.ConversationID, nowMS, nowMS); err != nil {
		return fmt.Errorf("upsert conversation: %w", err)
	}

	var tenantID, conversationID string
	err = tx.QueryRowContext(ctx, s.bind(`SELECT tenant_id, conversation_id FROM runs WHERE request_id = ?`), record.RequestID).Scan(&tenantID, &conversationID)
	switch {
	case err == nil:
		if tenantID != record.TenantID || conversationID != record.ConversationID {
			return conversation.ErrIdempotencyConflict
		}
		return tx.Commit()
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("read existing run: %w", err)
	}

	_, err = tx.ExecContext(ctx, s.bind(`
		INSERT INTO runs (
			request_id, tenant_id, conversation_id, status, error_code,
			authenticated, scope_count, nivora_version, nivora_commit,
			prompt_version, prompt_source, started_at_ms, finished_at_ms
		) VALUES (?, ?, ?, 'running', '', ?, ?, ?, ?, ?, ?, ?, 0)
	`),
		record.RequestID, record.TenantID, record.ConversationID,
		boolInt(record.Authenticated), record.ScopeCount,
		record.NivoraVersion, record.NivoraCommit,
		record.PromptVersion, record.PromptSource, nowMS,
	)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return tx.Commit()
}

func (s *Store) AppendMessage(ctx context.Context, record conversation.MessageRecord) error {
	if record.MessageID == "" || record.RequestID == "" || record.TenantID == "" || record.ConversationID == "" {
		return errors.New("message identity is required")
	}
	if record.Role != "user" && record.Role != "assistant" {
		return errors.New("message role must be user or assistant")
	}
	_, err := s.db.ExecContext(ctx, s.bind(`
		INSERT INTO messages (
			message_id, request_id, tenant_id, conversation_id, role, content, created_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (message_id) DO NOTHING
	`), record.MessageID, record.RequestID, record.TenantID, record.ConversationID, record.Role, record.Content, millis(record.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	var requestID, tenantID, conversationID, role, content string
	if err := s.db.QueryRowContext(ctx, s.bind(`
		SELECT request_id, tenant_id, conversation_id, role, content
		FROM messages WHERE message_id = ?
	`), record.MessageID).Scan(&requestID, &tenantID, &conversationID, &role, &content); err != nil {
		return fmt.Errorf("verify message: %w", err)
	}
	if requestID != record.RequestID || tenantID != record.TenantID || conversationID != record.ConversationID || role != record.Role || content != record.Content {
		return conversation.ErrIdempotencyConflict
	}
	_, err = s.db.ExecContext(ctx, s.bind(`
		UPDATE conversations SET updated_at_ms = ?
		WHERE tenant_id = ? AND conversation_id = ?
	`), millis(record.CreatedAt), record.TenantID, record.ConversationID)
	return err
}

func (s *Store) ToolStarted(ctx context.Context, record conversation.ToolAuditRecord) error {
	if err := validateTool(record); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.bind(`
		INSERT INTO tool_audits (
			request_id, tool_call_id, tenant_id, conversation_id,
			tool_name, status, reference_id, started_at_ms, finished_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, '', ?, 0)
		ON CONFLICT (request_id, tool_call_id)
		DO UPDATE SET tool_name = excluded.tool_name,
			status = excluded.status,
			started_at_ms = CASE
				WHEN tool_audits.started_at_ms = 0 THEN excluded.started_at_ms
				ELSE tool_audits.started_at_ms
			END
	`), record.RequestID, record.ToolCallID, record.TenantID, record.ConversationID, record.ToolName, record.Status, millis(record.StartedAt))
	if err != nil {
		return fmt.Errorf("record tool start: %w", err)
	}
	return nil
}

func (s *Store) ToolFinished(ctx context.Context, record conversation.ToolAuditRecord) error {
	if err := validateTool(record); err != nil {
		return err
	}
	finishedMS := millis(record.FinishedAt)
	startedMS := millis(record.StartedAt)
	_, err := s.db.ExecContext(ctx, s.bind(`
		INSERT INTO tool_audits (
			request_id, tool_call_id, tenant_id, conversation_id,
			tool_name, status, reference_id, started_at_ms, finished_at_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (request_id, tool_call_id)
		DO UPDATE SET tool_name = excluded.tool_name,
			status = excluded.status,
			reference_id = excluded.reference_id,
			finished_at_ms = excluded.finished_at_ms
	`), record.RequestID, record.ToolCallID, record.TenantID, record.ConversationID, record.ToolName, record.Status, record.ReferenceID, startedMS, finishedMS)
	if err != nil {
		return fmt.Errorf("record tool finish: %w", err)
	}
	if record.ReferenceID != "" {
		_, err = s.db.ExecContext(ctx, s.bind(`
			INSERT INTO support_case_refs (
				tenant_id, provider_case_id, conversation_id, status, created_at_ms, updated_at_ms
			) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id, provider_case_id)
			DO UPDATE SET conversation_id = excluded.conversation_id,
				status = excluded.status,
				updated_at_ms = excluded.updated_at_ms
		`), record.TenantID, record.ReferenceID, record.ConversationID, record.Status, finishedMS, finishedMS)
		if err != nil {
			return fmt.Errorf("record support case reference: %w", err)
		}
	}
	return nil
}

func (s *Store) FinishRun(ctx context.Context, finish conversation.RunFinish) error {
	if finish.RequestID == "" {
		return errors.New("request_id is required")
	}
	result, err := s.db.ExecContext(ctx, s.bind(`
		UPDATE runs SET status = ?, error_code = ?, finished_at_ms = ?
		WHERE request_id = ?
	`), finish.Status, finish.ErrorCode, millis(finish.FinishedAt), finish.RequestID)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Transcript(ctx context.Context, tenantID, conversationID string) ([]conversation.MessageRecord, error) {
	tenantID = strings.TrimSpace(tenantID)
	conversationID = strings.TrimSpace(conversationID)
	if tenantID == "" || conversationID == "" {
		return nil, errors.New("tenant and conversation are required")
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT message_id, request_id, tenant_id, conversation_id, role, content, created_at_ms
		FROM messages
		WHERE tenant_id = ? AND conversation_id = ?
		ORDER BY created_at_ms ASC, message_id ASC
	`), tenantID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query transcript: %w", err)
	}
	defer rows.Close()
	var records []conversation.MessageRecord
	for rows.Next() {
		var record conversation.MessageRecord
		var createdMS int64
		if err := rows.Scan(&record.MessageID, &record.RequestID, &record.TenantID, &record.ConversationID, &record.Role, &record.Content, &createdMS); err != nil {
			return nil, err
		}
		record.CreatedAt = time.UnixMilli(createdMS).UTC()
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) DeleteBefore(ctx context.Context, before time.Time) (conversation.RetentionResult, error) {
	cutoff := millis(before)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return conversation.RetentionResult{}, err
	}
	defer tx.Rollback()
	var result conversation.RetentionResult
	if result.ToolAudits, err = execDelete(ctx, tx, s.bind(`DELETE FROM tool_audits WHERE finished_at_ms > 0 AND finished_at_ms < ?`), cutoff); err != nil {
		return result, err
	}
	if result.Messages, err = execDelete(ctx, tx, s.bind(`DELETE FROM messages WHERE created_at_ms < ?`), cutoff); err != nil {
		return result, err
	}
	if result.Runs, err = execDelete(ctx, tx, s.bind(`DELETE FROM runs WHERE finished_at_ms > 0 AND finished_at_ms < ?`), cutoff); err != nil {
		return result, err
	}
	if result.SupportCases, err = execDelete(ctx, tx, s.bind(`DELETE FROM support_case_refs WHERE updated_at_ms < ?`), cutoff); err != nil {
		return result, err
	}
	if result.Conversations, err = execDelete(ctx, tx, s.bind(`
		DELETE FROM conversations
		WHERE updated_at_ms < ?
		AND NOT EXISTS (
			SELECT 1 FROM runs
			WHERE runs.tenant_id = conversations.tenant_id
			AND runs.conversation_id = conversations.conversation_id
		)
		AND NOT EXISTS (
			SELECT 1 FROM messages
			WHERE messages.tenant_id = conversations.tenant_id
			AND messages.conversation_id = conversations.conversation_id
		)
	`), cutoff); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) Check(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Close() error                    { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at_ms BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS conversations (
			tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			created_at_ms BIGINT NOT NULL,
			updated_at_ms BIGINT NOT NULL,
			PRIMARY KEY (tenant_id, conversation_id)
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			request_id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			status TEXT NOT NULL,
			error_code TEXT NOT NULL,
			authenticated INTEGER NOT NULL,
			scope_count INTEGER NOT NULL,
			nivora_version TEXT NOT NULL,
			nivora_commit TEXT NOT NULL,
			prompt_version TEXT NOT NULL,
			prompt_source TEXT NOT NULL,
			started_at_ms BIGINT NOT NULL,
			finished_at_ms BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS runs_tenant_conversation_idx ON runs (tenant_id, conversation_id, started_at_ms)`,
		`CREATE TABLE IF NOT EXISTS messages (
			message_id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at_ms BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS messages_tenant_conversation_idx ON messages (tenant_id, conversation_id, created_at_ms)`,
		`CREATE TABLE IF NOT EXISTS tool_audits (
			request_id TEXT NOT NULL,
			tool_call_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			status TEXT NOT NULL,
			reference_id TEXT NOT NULL,
			started_at_ms BIGINT NOT NULL,
			finished_at_ms BIGINT NOT NULL,
			PRIMARY KEY (request_id, tool_call_id)
		)`,
		`CREATE INDEX IF NOT EXISTS tool_audits_tenant_conversation_idx ON tool_audits (tenant_id, conversation_id, started_at_ms)`,
		`CREATE TABLE IF NOT EXISTS support_case_refs (
			tenant_id TEXT NOT NULL,
			provider_case_id TEXT NOT NULL,
			conversation_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at_ms BIGINT NOT NULL,
			updated_at_ms BIGINT NOT NULL,
			PRIMARY KEY (tenant_id, provider_case_id)
		)`,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply conversation migration: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, s.bind(`
		INSERT INTO schema_migrations (version, applied_at_ms)
		VALUES (1, ?)
		ON CONFLICT (version) DO NOTHING
	`), time.Now().UTC().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) bind(query string) string {
	if s.dialect != "pgx" {
		return query
	}
	var builder strings.Builder
	index := 1
	for _, character := range query {
		if character == '?' {
			fmt.Fprintf(&builder, "$%d", index)
			index++
		} else {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func validateRun(record conversation.RunRecord) error {
	if record.RequestID == "" || record.TenantID == "" || record.ConversationID == "" {
		return errors.New("run identity is required")
	}
	return nil
}

func validateTool(record conversation.ToolAuditRecord) error {
	if record.RequestID == "" || record.TenantID == "" || record.ConversationID == "" || record.ToolCallID == "" || record.ToolName == "" {
		return errors.New("tool audit identity is required")
	}
	return nil
}

func millis(value time.Time) int64 {
	if value.IsZero() {
		return time.Now().UTC().UnixMilli()
	}
	return value.UTC().UnixMilli()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func execDelete(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
