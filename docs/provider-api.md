# Provider API v1

Nivora never reads a product database directly. The product backend owns authentication, authorization, business rules, redaction, and writes. Nivora calls the following internal endpoints through a provider adapter.

Every request may include:

- `Authorization: Bearer <short-lived customer context>`
- `X-Nivora-Provider-Key: <service-to-service secret>`

The provider must validate both where applicable and must derive the customer identity from the signed context rather than from query parameters.

## Endpoints

- `GET /api/internal/support/capabilities`
- `GET /api/internal/support/context`
- `GET /api/internal/support/knowledge?q=...&limit=6`
- `GET /api/internal/support/resources?limit=10&status=...`
- `GET /api/internal/support/diagnosis?resource_id=...`
- `GET /api/internal/support/transactions?resource_id=...&limit=10`
- `POST /api/internal/support/cases`

List responses use `{ "items": [...] }`. Error responses should not contain raw stack traces, credentials, internal prompts, or hidden product fields.

## Capability names

The initial portable capability vocabulary is:

- `knowledge.search`
- `customer.context.read`
- `resource.list`
- `resource.diagnose`
- `transaction.read`
- `case.create`

A future Nivora release will use the capability response to register only supported tools. The first implementation keeps the contract explicit while the Lumio adapter is being built.
