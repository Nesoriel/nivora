# Durable conversation and audit storage

Nivora owns customer-support conversation state, but never reads or copies a product's business database. Product facts remain behind the Provider API.

## Stored data

- conversation IDs and tenant IDs;
- public user questions and final assistant answers;
- Agent run status, Nivora version/commit, and active Prompt version/source;
- sanitized Tool lifecycle records containing only Tool name, Tool Call ID, status, and timestamps;
- Provider support-case ID and status for human handoff.

Nivora never persists:

- model chain of thought or hidden reasoning;
- bearer contexts, API keys, or service secrets;
- Tool arguments or unrestricted Tool results;
- raw Provider payloads;
- product-internal prompts, generation recipes, or other customers' data.

## Drivers

Development and single-node testing may use SQLite:

```env
NIVORA_STORAGE_DRIVER=sqlite
NIVORA_STORAGE_DSN=file:/var/lib/nivora/nivora.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)
```

Production should use PostgreSQL:

```env
NIVORA_STORAGE_DRIVER=pgx
NIVORA_STORAGE_DSN=postgres://nivora:password@db.internal:5432/nivora?sslmode=require
NIVORA_STORAGE_REQUIRED=true
```

When storage is enabled it participates in `/readyz`. When `NIVORA_STORAGE_REQUIRED=true`, missing storage configuration prevents startup readiness.

## Idempotency

- `request_id` is the primary key for an Agent run.
- user and assistant message IDs are deterministic: `<request_id>:user` and `<request_id>:assistant`.
- Tool audits use `(request_id, tool_call_id)`.
- Provider case references use `(tenant_id, provider_case_id)`.
- replaying the same identity is a no-op;
- reusing an identity for another tenant or conversation returns an idempotency conflict.

Provider `case.create` remains protected by its separate stable idempotency key. If the Provider creates a case but the local audit write fails, the customer run fails closed; a retry must return the same Provider case before the local reference is recorded.

## Tenant isolation

Every transcript query requires both the configured tenant ID and conversation ID. The private API does not accept a browser-selected tenant:

```http
GET /v1/conversations/{conversation_id}/transcript
X-Nivora-Key: <internal service key>
```

Only public user/assistant messages are returned. Tool audit records and Provider payloads are never part of the transcript.

## Retention

```env
NIVORA_STORAGE_RETENTION=720h
NIVORA_STORAGE_CLEANUP_INTERVAL=1h
```

Cleanup is transactional and restart-safe. It removes completed runs, public messages, Tool audits, old support-case references, and finally conversations that no longer have retained records.

For regulated deployments, set retention according to the product's privacy policy and legal requirements. Database backups must use the same encryption, access control, deletion, and retention policy as the live database.

## Migration and rollback

Schema migrations run at startup inside a transaction and are recorded in `schema_migrations`.

Before deployment:

1. back up the database;
2. test the migration against a production-sized copy;
3. deploy Nivora with traffic disabled;
4. verify `/readyz` and transcript reads;
5. enable shadow traffic;
6. keep the previous binary available for rollback.

The initial schema is additive. Rolling back the binary does not require immediately deleting the new tables.
