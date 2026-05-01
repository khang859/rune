# Code Review Remediation Plan

This document captures the follow-up plan from the in-depth code review performed on 2026-05-01. It is intentionally prioritized around correctness, privacy/security, cancellation, and user-visible reliability.

## Status

Review completed for:

- CLI/TUI/editor behavior
- Provider clients, OAuth/config/secrets, SSE parsing, retry/cancellation
- Session persistence

Pending at the time this plan was written:

- Agent/tools/subagent-focused review task was still running. Fold any additional findings into this document before treating the plan as final.

Validation observed during review:

```sh
go test -race ./...
go vet ./...
```

Both passed locally. `staticcheck` was not installed in the review environment.

## Goals

- Prevent sessions or prompts from being sent to the wrong provider.
- Protect locally persisted sensitive data by default.
- Make cancellation and shutdown behavior reliable.
- Avoid duplicated or stuck provider streams.
- Keep the TUI responsive under shell shortcuts, file picking, and external startup work.
- Add regression tests for each fixed behavior.

## Non-goals

- Large architectural rewrites.
- Changing provider APIs beyond what is required for correctness and safety.
- Adding new user-facing features unrelated to the reviewed issues.

## Phase 1: Fix provider/session correctness and local privacy

**Status:** Completed on 2026-05-01.

Implemented:

- Session provider normalization now preserves `""`, `"codex"`, `"groq"`, `"ollama"`, and `"runpod"`; unknown provider values still normalize to `"codex"` for legacy compatibility.
- Session saves create/chmod the session directory with `0700`, write temp files with `0600`, and chmod the final session file to `0600` after rename.
- `EnsureRuneDir` creates and migrates `RuneDir()` and `SessionsDir()` to `0700`.
- Secrets and OAuth auth stores create/chmod parent directories with `0700` while preserving `0600` credentials, secret, and lock files.
- Added regression tests for provider round-trips, legacy unknown provider normalization, private session file/dir permissions, directory permission migration, and auth/secrets parent permissions.

Validation completed:

```sh
go test ./internal/session ./internal/config ./internal/ai/oauth
```

### 1. Preserve all supported provider IDs in persisted sessions

**Problem:** `internal/session/persist.go` normalizes anything except `groq` and `ollama` to `codex`, so Runpod and no-provider sessions can resume as Codex.

**Files:**

- `internal/session/persist.go`
- `internal/session/persist_test.go`

**Plan:**

- Update provider normalization to preserve:
  - `""`
  - `"codex"`
  - `"groq"`
  - `"ollama"`
  - `"runpod"`
- Decide whether unknown providers should fail closed to `""` or continue defaulting to `"codex"`. Recommended: keep `"codex"` for legacy unknown values, but preserve empty provider explicitly.
- Add save/load round-trip tests for all supported providers and empty provider.

**Verify:**

```sh
go test ./internal/session
```

### 2. Store session files with private permissions

**Problem:** sessions contain prompts, tool outputs, file contents, image/document data, assistant responses, and subagent summaries. They are currently created with default `os.Create` permissions, commonly resulting in `0644`.

**Files:**

- `internal/session/persist.go`
- `internal/session/persist_test.go`

**Plan:**

- Replace `os.Create(tmp)` with `os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)`.
- After rename, optionally `chmod` the final session file to `0600` to migrate existing files on write.
- Keep existing temp-file write, sync, close, and rename flow.
- Add a test asserting saved session files are not group/world-readable.

**Verify:**

```sh
go test ./internal/session
```

### 3. Create sensitive rune directories with private permissions

**Problem:** `~/.rune` and `~/.rune/sessions` are created as `0755`, leaking metadata about auth, secrets, session files, and mtimes.

**Files:**

- `internal/config/paths.go`
- `internal/config/paths_test.go`
- `internal/config/secrets.go`
- `internal/ai/oauth/store.go`

**Plan:**

- Make `EnsureRuneDir` create `RuneDir()` and `SessionsDir()` with `0700`.
- Ensure secrets/auth parent directories are created with `0700`.
- Consider `chmod` migration for existing directories when touched.
- Preserve `0600` file permissions for `auth.json`, lock files, and `secrets.json`.

**Verify:**

```sh
go test ./internal/config ./internal/ai/oauth
```

## Phase 2: Make CLI/TUI cancellation and terminal cleanup reliable

### 4. Use signal-aware root context

**Problem:** `cmd/rune/main.go` uses `context.Background()`, so prompt/login/MCP startup do not receive SIGINT/SIGTERM cancellation through context.

**Files:**

- `cmd/rune/main.go`
- relevant tests under `cmd/rune`

**Plan:**

- Use `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
- `defer stop()` in `main`.
- Thread this context through existing login, prompt, script, and interactive paths.
- Add targeted tests where practical for canceled prompt/script/login contexts.

**Verify:**

```sh
go test ./cmd/rune
```

### 5. Add bounded MCP startup timeouts

**Problem:** MCP startup, initialize, and tool listing happen before the TUI is available and can hang with a non-responsive server.

**Files:**

- `cmd/rune/interactive.go`
- `internal/mcp/manager.go`
- `internal/mcp/client.go`
- `internal/mcp/manager_test.go`

**Plan:**

- Add per-server timeout around connect/initialize/list operations.
- Continue recording failed server status rather than blocking startup.
- Consider connecting servers concurrently if sequential startup remains slow.
- Add a test with a fake non-responsive MCP server.

**Verify:**

```sh
go test ./internal/mcp ./cmd/rune
```

### 6. Guarantee terminal cleanup for Kitty keyboard mode

**Problem:** `RunWithProfile` only pops Kitty keyboard mode after normal `p.Run()` return. A panic can skip cleanup.

**Files:**

- `internal/tui/tui.go`
- possibly `cmd/rune/main.go`

**Plan:**

- Wrap cleanup in a `defer` immediately after constructing/running the program:

  ```go
  defer fmt.Print(ansi.PopKittyKeyboard(1))
  _, err := p.Run()
  return err
  ```

- Consider a panic-recovery cleanup hook in `main` for known terminal modes.
- If extracted into a helper, add unit coverage for cleanup invocation.

**Verify:**

```sh
go test ./internal/tui
```

## Phase 3: Keep the TUI responsive and shell behavior predictable

### 7. Run shell shortcuts asynchronously and make them cancellable

**Problem:** `!cmd` and `!!cmd` execute synchronously in the Bubble Tea update path with `context.Background()`, freezing the UI and ignoring command errors.

**Files:**

- `internal/tui/root.go`
- `internal/tui/events.go`
- `internal/tui/editor/shell.go`
- `internal/tui/*_test.go`

**Plan:**

- Convert shell execution to a `tea.Cmd` returning a shell-result message.
- Track shell execution state and cancellation in `RootModel`.
- Cancel shell execution from the existing Ctrl+C path.
- Bound captured output.
- Surface non-zero exit status and cancellation clearly.
- For `!cmd`, include command error/output in the prompt text sent to the model.

**Verify:**

```sh
go test ./internal/tui ./internal/tui/editor
```

### 8. Define attachment semantics for shell shortcuts

**Problem:** normal submissions drain image attachments; shell shortcuts do not, so attachments can leak into the next prompt.

**Files:**

- `internal/tui/editor/editor.go`
- `internal/tui/editor/editor_test.go`
- `internal/tui/root.go`

**Plan:**

- Recommended behavior:
  - `!cmd` drains pending attachments and includes them with the shell-generated prompt.
  - `!!cmd` preserves pending attachments and clearly does not consume them, or discards them with an explicit notice. Prefer preserving to avoid data loss.
- Update editor result construction accordingly.
- Add tests for pending attachments with both shell modes.

**Verify:**

```sh
go test ./internal/tui/editor ./internal/tui
```

### 9. Reject empty shell shortcuts

**Problem:** `!` and `!!` execute `bash -lc ""`, clear the editor, and may send empty shell-output prompts.

**Files:**

- `internal/tui/editor/editor.go`
- `internal/tui/editor/editor_test.go`

**Plan:**

- Require `strings.TrimSpace(cmd) != ""` before returning shell intent.
- If empty, keep the editor text or show a validation result without running shell.
- Update existing tests that currently classify `!` and `!!` as shell modes.

**Verify:**

```sh
go test ./internal/tui/editor
```

### 10. Avoid synchronous file-picker scans during key handling

**Problem:** typing `@` constructs a file picker and walks the repository synchronously.

**Files:**

- `internal/tui/editor/editor.go`
- `internal/tui/editor/filepicker.go`
- `internal/tui/editor/filepicker_test.go`

**Plan:**

- Make file scanning asynchronous and cancellable, with a loading state.
- Cache results per cwd with an explicit refresh or short TTL.
- Apply the same scan cap to hidden-file/dot-query paths.
- Add tests for scan limits and hidden query behavior.

**Verify:**

```sh
go test ./internal/tui/editor
```

### 11. Preserve suffix text when replacing file references

**Problem:** selecting a file reference replaces from the current token to EOF, discarding trailing text.

**Files:**

- `internal/tui/editor/editor.go`
- `internal/tui/editor/editor_test.go`

**Plan:**

- Preserve suffix at minimum:

  ```go
  e.ta.SetValue(val[:idx] + s + val[idx+len(cur):])
  ```

- Prefer cursor-aware token-span replacement if the textarea API exposes enough cursor information.
- Add tests for completing in the middle of a sentence.

**Verify:**

```sh
go test ./internal/tui/editor
```

### 12. Restrict slash menu activation to command position

**Problem:** slash menu opens on `/` at the start of any line in a multi-line prompt.

**Files:**

- `internal/tui/editor/editor.go`
- `internal/tui/editor/editor_test.go`

**Plan:**

- Only open slash menu when the command prefix is at the start of the prompt, not later lines.
- A practical rule: slash command mode only when the buffer before cursor has no newline and starts with `/`.
- Add tests for multi-line prompts containing `/tmp` or `/path` on later lines.

**Verify:**

```sh
go test ./internal/tui/editor
```

## Phase 4: Harden provider streaming and OAuth behavior

### 13. Prevent duplicated partial output across provider retries

**Problem:** provider-level retries wrap the whole streaming request, but partial SSE events are emitted immediately. A retry after partial output can duplicate text/tool calls.

**Files:**

- `internal/ai/codex/codex.go`
- `internal/ai/groq/groq.go`
- `internal/ai/ollama/ollama.go`
- `internal/ai/runpod/runpod.go`
- provider tests

**Plan:**

- Track whether any event was emitted in an attempt.
- Retry only if the attempt failed before emitting any event.
- Alternatively, buffer all events per attempt and release them only after stream success; this is more invasive.
- Add tests where attempt one emits partial text then fails transiently and attempt two succeeds.

**Verify:**

```sh
go test ./internal/ai/codex ./internal/ai/groq ./internal/ai/ollama ./internal/ai/runpod
```

### 14. Make stream-level provider errors terminal

**Problem:** SSE payload error frames emit `ai.StreamError` but parsers may keep reading unless the server closes.

**Files:**

- `internal/ai/codex/sse.go`
- `internal/ai/groq/sse.go`
- `internal/ai/runpod/sse.go`
- `internal/ai/ollama/sse.go` if applicable
- provider SSE tests

**Plan:**

- Choose one provider contract:
  - preferred: return classified parser errors for terminal provider error frames, or
  - emit `StreamError` and immediately terminate parsing.
- Ensure retry classification still works for retryable stream payload errors.
- Add tests for error frame followed by a never-ending stream.

**Verify:**

```sh
go test ./internal/ai/codex ./internal/ai/groq ./internal/ai/ollama ./internal/ai/runpod
```

### 15. Add streaming idle timeout or watchdog

**Problem:** providers use no HTTP client timeout for streaming, and SSE readers can block forever on stalled streams.

**Files:**

- provider clients/parsers under `internal/ai/*`
- provider tests

**Plan:**

- Add a configurable idle timeout for no SSE bytes/events received.
- Implement with a per-request cancellable context and watchdog timer, or lower-level connection read deadlines.
- Keep ordinary long-running streams alive as long as events/heartbeats arrive.
- Add tests with a server that sends headers then stalls.

**Verify:**

```sh
go test ./internal/ai/...
```

### 16. Stop silently ignoring malformed important SSE JSON frames

**Problem:** known event types with malformed JSON are often ignored, causing generic downstream errors.

**Files:**

- provider SSE parsers and tests

**Plan:**

- Return protocol errors for malformed known/important event payloads.
- Continue ignoring unknown event types for compatibility.
- Add tests for malformed text, tool-call, completion, and error frames.

**Verify:**

```sh
go test ./internal/ai/...
```

### 17. Add OAuth token timeouts and bounded/redacted error bodies

**Problem:** token exchange uses `http.DefaultClient` without timeout and returns full response bodies in errors.

**Files:**

- `internal/ai/oauth/codex.go`
- `internal/ai/oauth/codex_test.go`

**Plan:**

- Use a finite-timeout HTTP client or wrap requests with context timeout.
- Limit error bodies, e.g. `io.LimitReader(resp.Body, 8<<10)`.
- Redact token-like JSON fields before returning errors.
- Add tests for large error body and canceled/timeout context.

**Verify:**

```sh
go test ./internal/ai/oauth
```

### 18. Harden OAuth callback server

**Problem:** callback server has no read/header/idle timeouts and relies on callers to close it.

**Files:**

- `internal/ai/oauth/login.go`
- `internal/ai/oauth/login_test.go`

**Plan:**

- Set `ReadHeaderTimeout`, `ReadTimeout`, and `IdleTimeout` on the callback server.
- Consider closing automatically after a terminal callback result.
- Add tests if auto-close behavior is introduced.

**Verify:**

```sh
go test ./internal/ai/oauth
```

## Phase 5: Reconcile current subagent lifecycle changes

The working tree already contains uncommitted subagent shutdown/lifecycle changes in:

- `internal/agent/agent.go`
- `internal/agent/subagents.go`
- `internal/agent/subagents_test.go`
- `internal/tui/root.go`

Before merging those changes, address these review notes.

### 19. Preserve Plan/Act mode when refreshing system prompt

**Problem:** other agent replacement paths preserve `prevMode`; `refreshSystemPrompt` currently only preserves reasoning effort.

**File:**

- `internal/tui/root.go`

**Plan:**

- Save `prevMode := m.agent.Mode()` before replacing the agent.
- Call `m.agent.SetMode(prevMode)` after replacement.
- Ensure footer mode remains correct.
- Add or update TUI test coverage for `/reload` while in Plan Mode.

**Verify:**

```sh
go test ./internal/tui
```

### 20. Use bounded contexts for old-agent shutdown

**Problem:** replacement paths call `go old.Shutdown(context.Background())`, so a stuck provider/tool can leave shutdown goroutines waiting indefinitely.

**Files:**

- `internal/tui/root.go`
- `internal/agent/subagents.go`
- `internal/agent/subagents_test.go`

**Plan:**

- Add a helper that shuts down old agents with a timeout, e.g. 2–5 seconds.
- Use it consistently in provider/session/system-prompt replacement paths.
- Consider marking running tasks as cancelled during shutdown if they do not drain before timeout, or document restore-time reconciliation.

**Verify:**

```sh
go test ./internal/agent ./internal/tui
```

## Phase 6: Durability polish

### 21. Fsync parent directories for atomic writes

**Problem:** session saves fsync temp file but not parent dir; auth/secrets write temp and rename without fsyncing file data or parent dir.

**Files:**

- `internal/session/persist.go`
- `internal/ai/oauth/store.go`
- `internal/config/secrets.go`

**Plan:**

- For auth and secrets at minimum:
  - open temp file
  - write
  - sync
  - close
  - rename
  - open and sync parent directory
- Consider applying the same helper to sessions.
- Keep file permissions private.

**Verify:**

```sh
go test ./internal/session ./internal/config ./internal/ai/oauth
```

## Suggested implementation order

1. Provider normalization and session provider round-trip tests.
2. Session file permissions and rune directory permissions.
3. Signal-aware root context and MCP startup timeout.
4. Terminal cleanup defer.
5. TUI shell async/cancel/bounded output.
6. Shell attachment and empty-command behavior.
7. Current subagent lifecycle follow-ups: preserve mode and bounded shutdown.
8. Provider retry/error/idle-timeout hardening.
9. OAuth timeout/error-body/callback hardening.
10. Editor file-picker/reference/slash-menu UX fixes.
11. Atomic-write durability polish.

## Final validation checklist

Run targeted tests after each phase, then full validation before merging:

```sh
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
```

Also manually verify:

- Resume a Runpod session and a no-provider session.
- Inspect new session file and directory permissions.
- Press Ctrl+C during `rune --prompt` and during login.
- Start rune with a hung MCP server configured.
- Run `!sleep 30` in the TUI and cancel it.
- Trigger a provider stream error and confirm no duplicate partial output.
