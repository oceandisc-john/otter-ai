# Otter-AI Release Notes — ALPHA-0.0.3

**Date:** April 12, 2026

## Highlights

This release replaces the fragile regex-based intent routing system with a robust **tool/function-calling architecture**, adds comprehensive **unit test coverage** across all packages, and resolves several deployment reliability issues.

---

## New Features

### Tool/Function-Calling Architecture
- Replaced the old regex-based intent detection (`detectGovernanceAction`, 12 compiled regexps, and ~400 lines of pattern matching) with an LLM-native tool-calling loop in `ProcessMessage`.
- The agent now exposes **7 tools** to the LLM: `get_health_status`, `get_last_memory`, `search_memories`, `compare_memories`, `list_governance_state`, `propose_rule`, and `vote_on_proposal`.
- Added `ToolDefinition`, `ToolCall`, and `ToolParameter` types to the LLM package with OpenAI-compatible function-calling schema generation.
- Tool dispatch is handled in a new dedicated file: `internal/agent/tools.go` (254 lines).
- The LLM decides which tools to call based on the user's message, with a configurable round limit (`MaxToolRounds = 5`).

### Container Health & Memory Introspection
- New `internal/agent/introspection.go` (267 lines) with container health snapshots from cgroup v2 metrics, memory comparison responses, and sanitization utilities.
- Health metadata is automatically attached to stored memories via `storeMemoryWithContext`.

### Idle Musing Improvements
- **Concurrency guard**: An `atomic.Bool` prevents overlapping musings when the previous one hasn't finished.
- **User message preemption**: Incoming `ProcessMessage` calls cancel any in-flight musing so the LLM backend is freed immediately for user requests.
- Timeout bumped from 45s → 120s → **180s** to accommodate slow model inference on `qwen3.5:latest`.

### OpenWebUI Tool Compatibility
- Auto-retry logic in the OpenWebUI provider: if a completion with tools returns an error, it retries without tools for models that don't support function calling.

---

## Bug Fixes

### Embedding 500 Errors (Fixed)
- OpenWebUI requires the `:latest` tag on model names. Changed `OTTER_LLM_EMBEDDING_MODEL` from `nomic-embed-text` to `nomic-embed-text:latest`.

### Context Deadline Exceeded on Musings (Fixed)
- `IdleMusingTimeout` was too short for the full pipeline (LLM completion + embedding + store). Bumped to 180s.
- Added concurrency guard to prevent tick overlap when a musing takes longer than the 2-minute interval.

### Musing Starving User Requests (Fixed)
- `ProcessMessage` now cancels in-flight musing contexts before making its own LLM calls, preventing user-visible timeouts caused by backend contention with low-priority background work.

---

## Code Quality

### Dead Code Removal (~400 lines removed from `agent.go`)
- Removed the entire old regex-based intent routing layer:
  - `VoteInstruction` type and all 12 compiled regexps
  - `handleRuleProposal`, `handleVote`, `formatProposalDetails`
  - `previewRuleProposal`, `previewVote`, `extractRuleBody`, `parseAndResolveVotes`
  - `detectGovernanceAction` (LLM-based intent classifier, now replaced by native tool calling)
  - `isGovernanceRelevant`, `isNamingRelevant`, `isRecallQuery`, `isLastMemoryQuery`, `getLastMemoryResponse`, `isGovernanceStateQuery`
- Removed `isMemoryComparisonQuery` and 2 compiled regexps from `introspection.go`.
- Removed unused imports (`encoding/json`, `regexp`, `strconv` from agent.go; `regexp` from introspection.go).
- Cleaned up `newTestAgentWithGovernance` helper (zero callers).

### Unit Test Coverage
- **15 new test files** added across all 8 packages:
  - `internal/agent/agent_test.go` (861 lines, 48 tests)
  - `internal/api/jwt_test.go`, `ratelimit_test.go`, `server_test.go`
  - `internal/config/config_test.go`
  - `internal/governance/governance_test.go`, `crypto_test.go`, `keystore_test.go`
  - `internal/llm/llm_test.go`
  - `internal/memory/memory_test.go`
  - `internal/plugins/plugins_test.go`
  - `internal/vectordb/sqlite_test.go`, `vectordb_test.go`
- All 8 packages pass: `go test ./... ✓`
- Removed 22 dead tests that referenced deleted functions.

### Debug Logging
- Added timing instrumentation around LLM completion calls and tool execution in the `ProcessMessage` loop.
- Added debug logging to the `Embed()` function (URL, model, input length, response status).

---

## Configuration Changes

| Setting | Old Value | New Value | Reason |
|---|---|---|---|
| `IdleMusingTimeout` | 45s | 180s | Accommodate slow Ollama inference |
| `LLMClientTimeout` | 120s | 120s | (unchanged) |
| `OTTER_LLM_EMBEDDING_MODEL` | `nomic-embed-text` | `nomic-embed-text:latest` | OpenWebUI requires `:latest` tag |

---

## File Summary

| File | Status | Lines Changed |
|---|---|---|
| `internal/agent/agent.go` | Modified | +390 / -448 |
| `internal/agent/tools.go` | **New** | +254 |
| `internal/agent/introspection.go` | **New** | +267 |
| `internal/agent/agent_test.go` | **New** | +861 |
| `internal/llm/llm.go` | Modified | +97 (tool types + schema builder) |
| `internal/llm/providers.go` | Modified | +147 (tool support + auto-retry + debug logging) |
| `internal/api/server.go` | Modified | +36 |
| `internal/api/ratelimit.go` | Modified | +28 |
| `internal/config/config.go` | Modified | +31 |
| `internal/governance/governance.go` | Modified | +402 |
| `internal/memory/memory.go` | Modified | +8 |

---

## Breaking Changes

None. The external API and chat interface are unchanged. The internal refactor from regex routing to tool calling is transparent to users.

---

## Known Issues

- `qwen3.5:latest` can take 20-30s per LLM call through OpenWebUI, making multi-round tool calls slow for complex queries.
- The OpenWebUI auto-retry (without tools) adds latency when the primary model doesn't support function calling.
