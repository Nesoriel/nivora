# Nivora

English | [简体中文](README_zh.md)

Nivora is a reusable, tenant-aware customer-support agent runtime written in Go. It uses Eino for agent orchestration and keeps product data behind a versioned Provider API.

Lumio is the first provider integration, but Nivora itself does not know about Lumio tables, NextAuth, credits, generation pipelines, or SQLite.

## Current foundation

- Eino `ChatModelAgent` with Volcengine Ark through the official `eino-ext` adapter
- provider-neutral tools for knowledge, customer context, resources, diagnosis, transactions, and human-support cases
- stable Server-Sent Events protocol
- private service authentication between the product BFF and Nivora
- forwarding of short-lived customer context to the product Provider API
- health, readiness, and version endpoints
- loopback-first production deployment example

## Architecture

```text
Browser
  -> Product BFF (session, tenant, brand, rate limit)
     -> Nivora :3100 (Eino runtime, private)
        -> Product Provider API (authorization and business truth)
           -> Product services and database
```

Nivora does not accept a provider URL from chat requests and does not connect to a product database. The configured provider remains the source of truth.

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
```

Chat requests must come from a trusted BFF:

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
    }
  }'
```

The stream uses named SSE events:

- `message.delta`
- `tool.started`
- `tool.finished`
- `done`
- `error`

Tool results are not forwarded to the browser. They remain inside the agent run.

## Security boundary

- Bind Nivora to `127.0.0.1` or a private VPC address.
- Do not expose port `3100` through a public reverse proxy.
- Use separate secrets for product-to-Nivora and Nivora-to-provider authentication.
- The Provider API must enforce customer ownership and redact internal fields.
- Nivora's first release only performs read operations plus `case.create`.

See [Provider API v1](docs/provider-api.md).

## Development

```bash
make fmt
make test
make vet
make build
```

## Roadmap

1. Build the Lumio Provider API adapter and BFF client.
2. Add capability-driven tool registration.
3. Add durable conversations, audit logs, and support cases in Nivora's own storage.
4. Add shadow and canary modes for safe production rollout.
5. Add Eino interrupt/resume for human approval of future high-risk actions.
