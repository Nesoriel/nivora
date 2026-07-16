# Customer-support evaluation

A production customer-support Agent needs a fixed regression dataset. Manual conversations and successful compilation cannot prove that Tool selection, refusal behavior, latency, and factual wording remain safe after model, prompt, or Provider changes.

Nivora includes `nivora-eval`, a black-box evaluator that calls the same private SSE API used by a product BFF. It observes only customer-visible answers, Tool lifecycle events, completion state, error codes, and latency. It never inspects chain of thought.

## Dataset format

Datasets use JSON Lines. Each line contains one synthetic scenario:

```json
{
  "id": "failed-resource-refund",
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
  },
  "expected": {
    "required_tools": [
      "list_customer_resources",
      "diagnose_resource",
      "list_transactions"
    ],
    "forbidden_substrings": [
      "我猜",
      "应该已经退款"
    ],
    "max_latency_ms": 60000
  }
}
```

Supported deterministic assertions:

- required and forbidden answer substrings
- required and forbidden Tool names
- maximum end-to-end latency
- whether an Agent error is allowed
- whether the SSE run completed

Use synthetic accounts and synthetic Provider data only. Do not place production customer conversations, bearer tokens, private prompts, or business secrets in a repository dataset.

## Run against staging

```bash
export NIVORA_EVAL_BASE_URL=http://127.0.0.1:3100
export NIVORA_EVAL_SHARED_SECRET='replace-with-staging-secret'
export NIVORA_EVAL_BEARER_TOKEN='short-lived-synthetic-customer-context'

go run ./cmd/nivora-eval \
  -dataset evals/support-regression.example.jsonl \
  -output artifacts/eval-results.jsonl
```

The process exits with status `1` when any case fails and status `2` for configuration or dataset errors.

## CozeLoop path

The JSONL result is intentionally stable so a later CozeLoop integration can attach model traces, prompt versions, token usage, and evaluator scores to the same case IDs. CozeLoop should extend this black-box gate rather than replace it: production approval still requires deterministic security and Tool assertions outside the model.

Recommended release comparison:

1. Run the fixed dataset against the current approved Nivora build.
2. Run the same dataset against the candidate build with identical synthetic Provider data.
3. Compare pass rate, Tool selection, latency percentiles, token cost, and CozeLoop evaluator scores.
4. Block rollout on any critical security or ownership regression, even when aggregate quality improves.
