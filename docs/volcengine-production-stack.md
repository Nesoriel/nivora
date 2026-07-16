# Volcengine production stack

Nivora uses Volcengine as replaceable infrastructure rather than embedding product business rules into one cloud workflow.

## Implemented in this phase

### Ark model endpoint pool

`ARK_CHAT_MODELS` accepts an ordered list of Ark inference endpoint IDs. Nivora uses the first endpoint as primary and falls back only when model generation or stream creation fails before output begins.

Nivora never switches models after partial SSE output or Tool execution has started. This prevents duplicated actions and mixed answers.

## Planned ecosystem integrations

### CozeLoop tracing and evaluation

Eino provides an official CozeLoop callback adapter. The intended integration is process-level callback registration with secrets provided only through the deployment environment.

CozeLoop should capture:

- Agent and model latency
- Tool selection and duration
- token usage
- Provider failure categories
- offline evaluation datasets and regression scores

Customer identifiers, raw bearer contexts, secrets, private generation recipes, and unrestricted Tool payloads must be redacted before export.

Prompt management should be introduced only after prompt versions are pinned and cached locally. Nivora must continue starting with a safe bundled prompt when the remote prompt service is unavailable.

### VikingDB knowledge retrieval

VikingDB is suitable for approved product knowledge, semantic retrieval, and hybrid recall. It should sit behind the product Provider or a dedicated knowledge service rather than becoming a direct dependency of the Agent core.

This preserves the Provider boundary:

```text
Nivora -> Provider knowledge API -> VikingDB / approved database
```

The Provider remains responsible for tenant isolation, document approval status, source redaction, freshness, and access control.

### TOS attachments

Screenshots, generated previews, and customer-supplied diagnostic files can be stored in TOS with short-lived signed URLs. Nivora should receive only support-safe metadata and temporary access URLs. Permanent public object URLs must not be placed in prompts.

### Managed logs and metrics

Nivora emits structured JSON logs and a Prometheus-compatible `/metrics` endpoint. These can be collected by the company's existing Volcengine observability stack. The first metric surface includes active runs, success/failure totals, overload rejection totals, readiness failures, and Tool call totals.

## Production acceptance gate

Before Lumio traffic is enabled, the following must pass in a non-production environment:

1. Provider contract and ownership tests.
2. Ark primary-to-backup failover tests.
3. overload and timeout tests at expected peak concurrency.
4. prompt-injection and cross-tenant access tests.
5. replay tests proving support-case idempotency.
6. offline customer-support evaluation with a fixed regression dataset.
7. shadow traffic comparison against the existing Lumio support path.

Passing CI alone is not sufficient for production acceptance.
