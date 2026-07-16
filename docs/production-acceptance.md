# Production acceptance gate

This suite prepares Nivora for production review. Passing repository CI is necessary but does not authorize Lumio traffic. Real acceptance must run in an isolated company environment with approved Volcengine credentials, production-like infrastructure, synthetic or consented-redacted data, and an exercised rollback path.

## 1. Deterministic security probes

Run against the candidate before any model traffic is enabled:

```bash
NIVORA_PROBE_SHARED_SECRET=... \
  go run ./cmd/nivora-probe \
  -base-url http://candidate.internal:3100 \
  -dataset evals/security-probes.example.jsonl
```

The example probes cover:

- missing and invalid service keys;
- cross-tenant requests;
- anonymous privileged scopes;
- authenticated principals without Provider context;
- bearer context attached to anonymous principals;
- unknown JSON fields and attempted Provider URL injection;
- system-role history injection.

Production gate: every critical probe passes. No untrusted value, stack trace, credential, Provider URL, or internal prompt is reflected.

## 2. Synthetic Provider environment

`nivora-test-provider` is for isolated acceptance only. It must never receive production customer traffic.

```bash
NIVORA_TEST_PROVIDER_SHARED_SECRET=provider-secret \
NIVORA_TEST_PROVIDER_BEARER_TOKEN=synthetic-context \
  go run ./cmd/nivora-test-provider
```

Point Nivora staging at `http://127.0.0.1:3120`. The synthetic Provider offers deterministic knowledge, a failed video, matching charge/refund transactions, and idempotent support-case creation.

Fault examples:

```env
# Fail the first two Provider requests with 429, then recover.
NIVORA_TEST_PROVIDER_FAILURE_STATUS=429
NIVORA_TEST_PROVIDER_FAILURE_COUNT=2

# Inject latency before every Provider response.
NIVORA_TEST_PROVIDER_FAULT_DELAY=3s
```

Repeat with 429, 502, 503, and 504. Verify bounded retries occur only for idempotent reads, `case.create` does not automatically retry, readiness recovers, and no duplicate case is created.

## 3. Customer-support regression

```bash
NIVORA_EVAL_SHARED_SECRET=... \
NIVORA_EVAL_BEARER_TOKEN=synthetic-context \
  go run ./cmd/nivora-eval \
  -base-url http://candidate.internal:3100 \
  -dataset evals/support-regression.example.jsonl
```

Production gate:

- all critical factual, refusal, tenant, and Prompt-injection cases pass;
- no required Tool is missing;
- no forbidden Tool is used;
- no unverified refund, balance, case ID, or business action is invented;
- CozeLoop traces show the approved Prompt and model versions without raw customer content or secrets.

## 4. Load and recovery

```bash
NIVORA_LOAD_SHARED_SECRET=... \
NIVORA_LOAD_BEARER_TOKEN=synthetic-context \
  go run ./cmd/nivora-load \
  -base-url http://candidate.internal:3100 \
  -requests 500 \
  -concurrency 20 \
  -minimum-success-rate 0.99
```

The command reports first-token and completion p50/p95/p99, success rate, and error distribution. During the test, scrape `/metrics` for:

- `nivora_agent_active_runs`;
- `nivora_agent_queue_rejected_total`;
- `nivora_agent_runs_failed_total`;
- `nivora_process_goroutines`;
- `nivora_process_heap_alloc_bytes`;
- `nivora_process_heap_objects`.

Do not copy example concurrency values into production. Establish limits from the actual CPU, memory, Ark endpoint quotas, Provider capacity, and agreed SLO.

Production gate:

- measured p95/p99 stay within the approved SLO;
- queue rejection is expected and bounded under deliberate overload;
- goroutine and heap usage return near baseline after traffic stops;
- no sustained memory growth appears across repeated runs;
- service recovers after Provider faults, Ark primary failure, process restart, and database interruption;
- traffic-disable and rollback procedures are exercised successfully.

## 5. Shadow comparison

Only synthetic or consented-redacted questions may be used. Candidate answers must not be shown to customers during shadow mode.

```bash
NIVORA_SHADOW_BASELINE_KEY=... \
NIVORA_SHADOW_CANDIDATE_KEY=... \
NIVORA_SHADOW_BASELINE_BEARER=... \
NIVORA_SHADOW_CANDIDATE_BEARER=... \
  go run ./cmd/nivora-shadow \
  -baseline-url http://baseline.internal:3100 \
  -candidate-url http://candidate.internal:3100 \
  -dataset approved-redacted-shadow.jsonl \
  -output shadow-results.jsonl
```

The output intentionally stores only answer hashes and byte counts, Tool sets, completion/error status, and latency. It does not write answer text.

Review:

- deterministic candidate pass rate;
- factual correctness and refusal quality;
- Tool selection differences;
- escalation and support-case rate;
- latency and token cost in CozeLoop;
- Ark endpoint failover rate;
- Provider error and retry distribution.

## 6. Release decision

A production release requires documented approval of:

1. exact Nivora commit, model endpoint IDs, Prompt version, Provider API version, and knowledge index version;
2. security probe results;
3. customer-support regression results;
4. load and recovery report;
5. shadow comparison report;
6. database backup/migration validation;
7. rollback owner, traffic-disable switch, and incident contacts.

Start with shadow traffic, then an internal allowlist, then a small read-only canary. High-risk writes remain disabled until a separate human-approval design and acceptance cycle are complete.
