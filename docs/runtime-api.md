# Nivora Runtime API v1

`POST /v1/chat/stream` is an internal API for a trusted product BFF. Browsers must not call it directly.

Required headers:

- `X-Nivora-Key: <product-to-Nivora service secret>`
- `Authorization: Bearer <short-lived Provider context>` for authenticated principals

Example authenticated request:

```json
{
  "question": "我的视频为什么失败，积分退了吗？",
  "conversation_id": "conv_123",
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
  },
  "history": []
}
```

The BFF must remove any browser-supplied `tenant` and `principal` values and inject trusted values after validating the product session. Nivora authenticates the BFF with `X-Nivora-Key`, while the Provider independently validates the short-lived bearer context.

## Scope vocabulary

- `knowledge:read`
- `customer:read`
- `resource:read`
- `transaction:read`
- `case:create`

Anonymous principals may receive only `knowledge:read` and `case:create`. Customer, resource, and transaction scopes require `authenticated: true` and a bearer context.

Nivora intersects scopes with Provider capabilities before constructing the Eino Agent. Missing Tools cannot be recovered through prompting or model choice.

## Overload behavior

Nivora accepts at most `NIVORA_MAX_CONCURRENT_RUNS` active runs. Requests that cannot acquire a slot before `NIVORA_QUEUE_TIMEOUT` receive:

```http
HTTP/1.1 503 Service Unavailable
Retry-After: 1
Content-Type: application/json

{"error":"service_busy"}
```

## Streaming behavior

Successful requests return named Server-Sent Events. Nivora emits heartbeat comments while a model or Tool is working so reverse proxies do not treat the connection as idle.

```text
event: message.delta
data: {"type":"message.delta","content":"..."}

: ping

event: done
data: {"type":"done"}
```

Supported event names:

- `message.delta`
- `tool.started`
- `tool.finished`
- `done`
- `error`

Raw Tool results, prompts, credentials, and Provider error bodies are never forwarded to the browser.
