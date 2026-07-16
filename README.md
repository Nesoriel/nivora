# Nivora

English | [简体中文](README.zh-CN.md)

Nivora is a reusable, tenant-aware customer-support Agent Runtime written in Go. It uses Eino for agent orchestration and keeps product data behind a versioned Provider API.

Lumio is the first planned provider integration, but Nivora itself does not know about Lumio tables, NextAuth, credits, generation pipelines, or SQLite.

> Nivora is an integration-ready runtime under active production hardening. It is not considered production-accepted until a real Provider, security evaluation, load test, and shadow-traffic evaluation have passed.

## Current foundation

- Eino `ChatModelAgent` with Volcengine Ark through the official `eino-ext` adapter
- ordered Ark endpoint failover before streaming begins
- capability- and scope-driven Tool registration
- provider-neutral Tools for knowledge, customer context, resources, diagnosis, transactions, and human-support cases
- bounded Provider retries for idempotent reads and idempotent support-case creation
- stable Server-Sent Events protocol with heartbeat comments
- private service authentication between the product BFF and Nivora
- real Provider readiness checks with short caching
- global concurrency and queue protection
- Prometheus-compatible runtime metrics
- loopback-first production deployment example

## Architecture

```text
Browser
  -> Product BFF (session, tenant, brand, scopes, rate limit)
     -> Nivora :3100 (Eino runtime, private)
        -> Product Provider API (authorization and business truth)
           -> Product services and database / approved knowledge service
```

Nivora does not accept a Provider URL from chat requests and does not connect to a product database. The configured Provider remains the source of truth.

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

- Bind Nivora to `127.0.0.1` or a private VPC address.
- Do not expose port `3100` through a public reverse proxy.
- Use separate secrets for product-to-Nivora and Nivora-to-Provider authentication.
- The Provider API must enforce customer ownership and redact internal fields.
- Anonymous requests can receive only explicitly granted knowledge and case scopes.
- Nivora currently performs read operations plus idempotent `case.create` only.

## Documentation

- [Runtime API v1](docs/runtime-api.md)
- [Provider API v1](docs/provider-api.md)
- [Volcengine production stack](docs/volcengine-production-stack.md)

## Development

```bash
make fmt
make test
make vet
make build
```

## Roadmap

1. Integrate CozeLoop tracing, prompt versioning, and offline evaluation with strict redaction.
2. Build a Provider-backed knowledge pipeline that can use VikingDB for approved semantic retrieval.
3. Add durable conversations, audit logs, and support cases in Nivora's own storage.
4. Add shadow and canary modes for safe production rollout.
5. Add Eino interrupt/resume for human approval of future high-risk actions.
