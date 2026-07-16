# Approved knowledge service

Nivora Core does not hold VikingDB credentials. A product Provider may deploy the reference `nivora-knowledge` service behind its own authorization boundary:

```text
Nivora search_knowledge Tool
  -> Product Provider API
     -> nivora-knowledge (private)
        -> VikingDB
```

The Provider derives `tenant_id` from its signed customer context. It must never accept an arbitrary tenant selected by the browser.

## Required VikingDB fields

The recommended collection uses platform vectorization for the `content` field and scalar indexes for the trust fields:

| Field | Type | Required | Purpose |
|---|---|---:|---|
| `content` | text | yes | Approved customer-visible chunk |
| `tenant_id` | string | yes | Hard tenant boundary |
| `document_id` | string | yes | Stable logical document ID |
| `chunk_id` | string | yes | Stable chunk ID |
| `source_title` | string | yes | Customer-visible citation title |
| `source_version` | string | yes | Approved document version |
| `source_uri` | string | no | Customer-visible support URL |
| `approval_status` | string | yes | Must equal `approved` |
| `effective_at` | int64 or RFC3339 string | yes | Version activation time |
| `expires_at` | int64 or RFC3339 string | no | Optional expiry time |

At minimum, create scalar indexes for `tenant_id` and `approval_status`. Indexing `effective_at` and `expires_at` is recommended for operational queries, although Nivora validates freshness again after retrieval.

## Search safety

The VikingDB adapter pushes this DSL into every query:

```json
{
  "op": "and",
  "conditions": [
    {"op": "term", "field": "tenant_id", "value": "resolved-tenant"},
    {"op": "term", "field": "approval_status", "value": "approved"}
  ]
}
```

The outer approved-knowledge service then rejects any result that has:

- a mismatched tenant;
- a status other than `approved`;
- an activation time in the future;
- an expiry time at or before the current time;
- missing document, chunk, title, version, or content fields;
- a relevance score below the service-level minimum;
- a duplicate document/chunk/version identity.

This second validation is mandatory. It protects customers when an index is misconfigured, stale, or polluted.

## Publication workflow

1. Ingest a document as `draft` with a new immutable `source_version`.
2. Chunk and index the full version.
3. Run retrieval tests for tenant isolation, source precision, and answerability.
4. Change all chunks of the version to `approved` in one controlled publication operation.
5. Keep the previous approved version available until the new version passes smoke checks.
6. Revoke or expire the previous version only after successful publication.
7. Re-index revoked documents immediately and verify that retrieval returns no chunks.

Never index private generation recipes, unrestricted internal prompts, credentials, customer data, or documents that have not passed the product's approval process.

## Service configuration

```env
NIVORA_KNOWLEDGE_ADDR=127.0.0.1:3110
NIVORA_KNOWLEDGE_SHARED_SECRET=replace-with-a-long-random-secret
NIVORA_KNOWLEDGE_MIN_SCORE=0.75
NIVORA_KNOWLEDGE_OVERSAMPLE=3
NIVORA_KNOWLEDGE_REQUEST_TIMEOUT=15s

VIKINGDB_HOST=api-vikingdb.volces.com
VIKINGDB_REGION=cn-beijing
VIKINGDB_AK=your-access-key
VIKINGDB_SK=your-secret-key
VIKINGDB_SCHEME=https
VIKINGDB_COLLECTION=nivora_support_knowledge
VIKINGDB_INDEX=approved_hybrid
VIKINGDB_PARTITION=default
VIKINGDB_CONNECTION_TIMEOUT_SECONDS=5
VIKINGDB_WITH_MULTIMODAL=true
VIKINGDB_USE_SPARSE=true
VIKINGDB_DENSE_WEIGHT=0.7
```

For platform-vectorized collections, keep `VIKINGDB_WITH_MULTIMODAL=true`. When disabled, configure `VIKINGDB_EMBEDDING_MODEL` for VikingDB built-in embedding.

## Private API

```http
POST /v1/search
X-Nivora-Knowledge-Key: <service secret>
Content-Type: application/json

{
  "tenant_id": "resolved-tenant",
  "query": "退款多久到账？",
  "limit": 6,
  "min_score": 0.8
}
```

The configured `NIVORA_KNOWLEDGE_MIN_SCORE` is a floor. A caller may request a higher threshold but cannot lower the service policy.
