# CozeLoop production integration

Nivora integrates CozeLoop through the official Eino callback adapter and the official CozeLoop Go SDK. The integration is optional and fail-open for customer traffic: tracing or PromptHub outages must not make customer conversations fail.

## Enable tracing

```env
NIVORA_COZELOOP_ENABLED=true
COZELOOP_WORKSPACE_ID=your-workspace-id
```

For local development, an API token can be used:

```env
COZELOOP_API_TOKEN=your-api-token
```

Production deployments should use JWT OAuth credentials instead of a long-lived PAT:

```env
COZELOOP_JWT_OAUTH_CLIENT_ID=your-client-id
COZELOOP_JWT_OAUTH_PRIVATE_KEY=your-private-key
COZELOOP_JWT_OAUTH_PUBLIC_KEY_ID=your-public-key-id
```

## Trace redaction

The Eino callback parser uses an allowlist. It may export:

- model provider and endpoint/model name
- input, output, and total token counts
- first-response latency
- stream flag
- Prompt key, version, and provider
- Tool call ID

It deliberately removes:

- customer questions and conversation history
- model answers and reasoning content
- Tool inputs and Tool outputs
- bearer contexts and service secrets
- raw Provider payloads
- product-internal prompts, recipes, and hidden metadata

The root Nivora run span contains only support-safe identifiers and release metadata: request ID, conversation ID, tenant ID, Nivora version/commit, active Prompt version/source, authentication state, scope count, and Tool count.

## Approved Prompt policy

Nivora's mandatory safety rules are compiled into the binary. CozeLoop PromptHub may provide only an additional approved system-policy appendix.

```env
NIVORA_COZELOOP_PROMPT_KEY=nivora.support.policy
NIVORA_COZELOOP_PROMPT_VERSION=v1.2.0
NIVORA_COZELOOP_PROMPT_REFRESH=5m
NIVORA_COZELOOP_PROMPT_TIMEOUT=3s
```

Production should pin an approved Prompt version. An empty version means "latest" and should be limited to controlled staging.

A remote Prompt is accepted only when it contains at least one non-empty system-role message and the combined appendix is at most 32 KiB. User-role-only Prompts are rejected.

Failure behavior:

1. Keep the most recent successfully fetched Prompt in memory.
2. If no remote Prompt has ever succeeded, use the bundled policy.
3. Never stop serving because PromptHub is unavailable.
4. Never allow a remote appendix to weaken the compiled safety rules.

## Diagnostics

`GET /version` reports:

- Nivora version and commit
- configured Prompt key
- active Prompt version and source
- whether CozeLoop initialized successfully

No credential or raw Prompt content is returned.
