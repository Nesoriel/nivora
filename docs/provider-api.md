# Provider API v1

Nivora never reads a product database directly. The product backend owns authentication, authorization, business rules, redaction, and writes. Nivora calls the following internal endpoints through a Provider adapter.

Every request may include:

- `Authorization: Bearer <short-lived customer context>`
- `X-Nivora-Provider-Key: <service-to-service secret>`

The Provider must validate both where applicable and must derive the customer identity from the signed context rather than from query parameters. The service key and customer context have different trust purposes and must use different secrets.

## Endpoints

- `GET /api/internal/support/capabilities`
- `GET /api/internal/support/context`
- `GET /api/internal/support/knowledge?q=...&limit=6`
- `GET /api/internal/support/resources?limit=10&status=...`
- `GET /api/internal/support/diagnosis?resource_id=...`
- `GET /api/internal/support/transactions?resource_id=...&limit=10`
- `POST /api/internal/support/cases`

List responses use `{ "items": [...] }`. Error responses must not contain raw stack traces, credentials, internal prompts, private generation recipes, or hidden product fields.

## Readiness contract

`GET /api/internal/support/capabilities` must accept service authentication without customer context. Nivora uses it for readiness checks and expects a supported v1 version. The endpoint should be inexpensive and must not call an LLM.

A request with customer context may return a narrower capability list when the authenticated user, tenant, or product plan supports fewer operations.

## Capability names

The portable capability vocabulary is:

- `knowledge.search`
- `customer.context.read`
- `resource.list`
- `resource.diagnose`
- `transaction.read`
- `case.create`

Nivora intersects this list with the scopes injected by the trusted product BFF. A Tool is registered only when both the Provider capability and the request scope permit it.

## Retry and idempotency contract

Nivora may retry only idempotent Provider reads after network errors or HTTP `429`, `502`, `503`, and `504` responses. Providers should return the same support-safe representation for repeated reads.

`POST /api/internal/support/cases` is never automatically retried by Nivora. It includes an `Idempotency-Key` header derived from the conversation and case subject. The Provider must persist or otherwise honor this key so repeated model Tool calls return the original case rather than creating duplicates.

## Ownership rules

The Provider must perform ownership checks on every customer-specific endpoint, including when a resource identifier is valid. A valid identifier is not proof that the current customer owns the resource.
