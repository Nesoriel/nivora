# Nivora

English | [简体中文](README.zh-CN.md)

Nivora is a reusable, tenant-aware customer-support Agent Runtime written in Go. It uses Eino for Agent orchestration and keeps product data behind a versioned Provider API.

Lumio is the first planned provider integration, but Nivora itself does not know about Lumio tables, NextAuth, credits, generation pipelines, or SQLite.

> Nivora now includes the engineering and acceptance tooling needed to prepare a production candidate. It is not production-approved until the real Provider contract, Volcengine environment, load and recovery tests, and consented-redacted shadow comparison have passed in the company's isolated staging environment.

## Current foundation

- Eino `ChatModelAgent` with Volcengine Ark through the official `eino-ext` adapter
- ordered Ark endpoint failover before streaming begins
- optional CozeLoop tracing and PromptHub policy versions with strict trace redaction and bundled fallback
- capability- and scope-driven Tool registration
- provider-neutral Tools for knowledge, customer context, resources, diagnosis, transactions, and human-support cases
- Provider-side approved-knowledge reference service using the official Eino VikingDB Retriever
- tenant, approval, freshness, provenance, and score validation after semantic retrieval
- SQLite development and PostgreSQL production storage for public transcripts, run metadata, sanitized Tool audits, and support-case references
- deterministic replay protection and tenant-scoped transcript access
- black-box customer-support and knowledge-retrieval JSONL evaluation tools
- deterministic HTTP security probes for authentication, tenant, scope, request-shape, and history-injection boundaries
- SSE load testing with first-token and completion p50/p95/p99, success rate, and error distribution
- privacy-safe baseline/candidate shadow comparison using answer hashes instead of answer text
- a synthetic Provider with deterministic support facts, idempotent cases, latency, and 429/5xx fault injection
- a manual staging acceptance workflow for probes, regression, load, and shadow suites
- bounded Provider retries for idempotent reads and idempotent support-case creation
- stable Server-Sent Events protocol with heartbeat comments
- private service authentication between the product BFF and Nivora
- real Provider and storage readiness checks with short caching
- global concurrency and queue protection
- Prometheus-compatible Agent and process metrics
- loopback-first production deployment examples

## Architecture

```text
Browser
  -> Product BFF (session, tenant, brand, scopes, rate limit)
     -> Nivora :3100 (Eino runtime, private)
        -> Nivora conversation/audit database
        -> Product Provider API (authorization and business truth)
           -> Product services and database
           -> approved knowledge service :3110
              -> VikingDB
```

Nivora does not accept a Provider URL from chat requests and does not connect to a product business database or VikingDB. The configured Provider remains the source of truth.

## Run locally

```bash
cp .env.example .env
set -a; source .env; set +a
go run ./cmd/nivora
```

Useful endpoints:

```bash
curl http://127.0.0.1:3100/healthz
curl -i http://127.0.0.1:3100/readyz
curl http://127.0.0.1:3100/metrics
curl -H 'X-Nivora-Key: replace-with-a-long-random-secret' \
  http://127.0.0.1:3100/v1/conversations/conv-id/transcript
```

Chat requests must come from a trusted BFF. The BFF must replace browser-supplied tenant and principal data with trusted server-side values.

```bash
curl -N http://127.0.0.1:3100/v1/chat/stream \
  -H 'Content-Type: application/json' \
  -H 'X-Nivora-Key: replace-with-a-long-random-secret' \
  -H 'Authorization: Bearer short-lived-provider-context' \
  -d '{
    "question": "我的视频为什么失败，积分退了吗？",
    "tenant": {
      "id": "lumio",
      "brand": {
        "key": "lumio",
        "name": "Lumio",
        "agent_name": "Lumio 智能客服"
      }
    },
    "principal": {
      "authenticated": true,
      "scopes": [
        "knowledge:read",
        "customer:read",
        "resource:read",
        "transaction:read",
        "case:create"
      ]
    }
  }'
```

The stream uses named SSE events:

- `message.delta`
- `tool.started`
- `tool.finished`
- `done`
- `error`

Tool results are not forwarded to the browser. They remain inside the Agent run.

## Security boundary

- Bind Nivora and its reference services to loopback or private VPC addresses.
- Do not expose ports `3100`, `3110`, or the synthetic Provider through a public reverse proxy.
- Use separate secrets for product-to-Nivora, Nivora-to-Provider, and Provider-to-knowledge authentication.
- The Provider API must enforce customer ownership and redact internal fields.
- Anonymous requests can receive only explicitly granted knowledge and case scopes.
- Durable storage contains public messages and sanitized audit metadata only; it never stores chain of thought, bearer contexts, Tool payloads, or product recipes.
- Shadow output stores answer hashes and byte counts, not answer text.
- The synthetic Provider is for isolated acceptance only and must never serve production customer traffic.
- Nivora currently performs read operations plus idempotent `case.create` only.

## Production acceptance commands

```bash
make probe          # deterministic HTTP security boundaries
make eval           # customer-support regression
make knowledge-eval # approved knowledge retrieval benchmark
make load           # bounded SSE load and latency test
make shadow         # baseline/candidate privacy-safe comparison
make test-provider  # synthetic Provider for isolated staging
```

A manually triggered GitHub Actions workflow is available at `.github/workflows/acceptance.yml`. It expects protected `nivora-staging` environment variables and secrets; it does not run against production automatically.

## Documentation

- [Runtime API v1](docs/runtime-api.md)
- [Provider API v1](docs/provider-api.md)
- [CozeLoop integration](docs/cozeloop.md)
- [Approved VikingDB knowledge](docs/approved-knowledge.md)
- [Durable conversation storage](docs/durable-storage.md)
- [Customer-support evaluation](docs/evaluation.md)
- [Production acceptance gate](docs/production-acceptance.md)
- [Volcengine production stack](docs/volcengine-production-stack.md)

## Development

```bash
make fmt
make test
make vet
make build
```

## Roadmap

1. Run the complete acceptance gate in the company's isolated staging environment and establish approved SLO, quality, cost, and rollback baselines.
2. Add Eino interrupt/resume and explicit human approval before any future high-risk write Tool is introduced.
