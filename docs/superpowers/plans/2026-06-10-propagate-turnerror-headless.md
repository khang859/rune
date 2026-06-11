# Propagate Agent TurnError from Headless Runners Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Headless `--prompt` and `--script` runs return a non-nil error (and therefore a non-zero exit code) when the agent emits `agent.TurnError`, instead of printing `[error: ...]` and exiting 0.

**Architecture:** Both `runPrompt` and `runScript` consume `agent.Run`'s event channel. We capture the *first* `agent.TurnError` event into a local variable while preserving the existing transcript output, then return errors after the event loop in this strict order: (1) session save error, (2) captured TurnError, (3) `agent.ErrIncompleteRequiredTool` (prompt mode only), (4) nil. No changes to `main.go` — a returned TurnError flows through the existing generic-error path (exit 1), and `ErrIncompleteRequiredTool` keeps its dedicated exit 3.

**Tech Stack:** Go, stdlib `testing`, `httptest` for fake providers, `internal/ai/faux` scriptable provider.

**Issue:** https://github.com/khang859/rune/issues/27

---

## Background for the implementer (zero-context primer)

- `internal/agent/loop.go` runs one agent turn and emits events on a channel. Terminal failures arrive as `agent.TurnError{Err error}` (provider fatal errors, retry exhaustion, unexpected stream end, recovered panics). At most one `TurnError` is emitted per `Run` before the channel closes, but we defensively keep only the first.
- `cmd/rune/prompt.go` → `runPrompt` is the `--prompt` headless path. `cmd/rune/script.go` → `runScript` is the `--script` path (drives the `faux` fake provider from a JSON file).
- `cmd/rune/main.go:90-103` maps `runPrompt`'s error to exit codes: `agent.ErrIncompleteRequiredTool` → exit 3, any other non-nil error → exit 1. This mapping must keep working unchanged.
- **How to force a `TurnError` in tests:**
  - *Script mode:* the faux provider streams a turn's events then closes the stream. If a turn has a `Reply` but no `Done` step, the agent loop sees the stream end without an `ai.Done` event and emits `TurnError{Err: errors.New("stream ended unexpectedly")}` (loop.go:197).
  - *Prompt mode:* the ollama provider classifies HTTP 400 as `ai.ErrFatal` (`internal/ai/ollama/ollama.go:216-233`), which `classifyRetry` does not retry, so the loop emits `TurnError` with `status 400: ...` (loop.go:130).
- **How to force a session-save failure in tests:** `session.Save` writes `<path>.tmp` then `os.Rename(tmp, path)`. If `path` is an existing *directory*, the rename fails, so `Save` returns an error.
- Existing tests use `t.Setenv("RUNE_DIR", t.TempDir())` to sandbox config/sessions and `t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)` to point at an `httptest` server. Follow that pattern.
- Existing regression to be aware of: `TestRunPrompt_CanceledContextCancelsProviderRequest` expects `runPrompt` to return nil after context cancellation. That stays green because cancellation emits `TurnAborted`, not `TurnError`.

## File Structure

| File | Change |
|---|---|
| `cmd/rune/script.go` | Capture first `TurnError` in `runScript`; return it after the save check. |
| `cmd/rune/script_test.go` | Add 2 tests: TurnError propagation; save-error precedence over TurnError. |
| `cmd/rune/prompt.go` | Capture first `TurnError` in `runPrompt`; return order: save → TurnError → incomplete → nil. |
| `cmd/rune/prompt_test.go` | Add 1 test: TurnError propagation with transcript + session-save assertions. |

No new files. No changes to `main.go`, `internal/agent`, or `internal/ai`.

---

### Task 1: Propagate TurnError from `runScript`

**Files:**
- Modify: `cmd/rune/script.go:75-94`
- Test: `cmd/rune/script_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/rune/script_test.go` (imports already cover everything needed):

```go
func TestRunScript_TurnErrorReturnsError(t *testing.T) {
	dir := t.TempDir()
	sessPath := filepath.Join(dir, "s.json")

	// A turn with a reply but no Done step: the faux stream ends without an
	// ai.Done event, so the agent loop emits TurnError("stream ended unexpectedly").
	sc := scriptFile{
		Provider: "faux",
		Session:  sessPath,
		Model:    "gpt-5",
		Faux: []fauxStep{
			{Reply: "partial"},
		},
		UserMessage: "hi",
	}
	b, _ := json.Marshal(sc)
	scriptPath := filepath.Join(dir, "in.json")
	_ = os.WriteFile(scriptPath, b, 0o644)

	var out bytes.Buffer
	err := runScript(context.Background(), scriptPath, &out, faux.New())
	if err == nil || !strings.Contains(err.Error(), "stream ended unexpectedly") {
		t.Fatalf("err = %v, want stream-ended error", err)
	}
	if !strings.Contains(out.String(), "[error:") {
		t.Fatalf("transcript missing [error: ...] line: %q", out.String())
	}
	if _, statErr := os.Stat(sessPath); statErr != nil {
		t.Fatalf("session file not written before error return: %v", statErr)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/rune -run TestRunScript_TurnErrorReturnsError -v`

Expected: FAIL with `err = <nil>, want stream-ended error` (current code returns nil).

- [ ] **Step 3: Implement the capture in `runScript`**

In `cmd/rune/script.go`, replace the event loop and tail of the function (currently lines 74-94):

```go
	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: sc.UserMessage}}}
	var turnErr error
	for ev := range a.Run(ctx, msg) {
		switch v := ev.(type) {
		case agent.AssistantText:
			fmt.Fprint(w, v.Delta)
		case agent.ToolStarted:
			fmt.Fprintf(w, "\n[tool start: %s]", v.Call.Name)
		case agent.ToolFinished:
			fmt.Fprintf(w, "\n[tool done: %s -> %q]", v.Call.Name, truncate(v.Result.Output, 80))
		case agent.TurnError:
			if turnErr == nil {
				turnErr = v.Err
			}
			fmt.Fprintf(w, "\n[error: %v]", v.Err)
		case agent.TurnDone:
			fmt.Fprintf(w, "\n[done: %s]", v.Reason)
		}
	}
	if sc.Session != "" {
		if err := sess.Save(); err != nil {
			return fmt.Errorf("save session: %w", err)
		}
	}
	return turnErr
}
```

Only two things change: the `var turnErr error` declaration with the capture inside `case agent.TurnError`, and the final `return nil` becomes `return turnErr`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/rune -run TestRunScript_TurnErrorReturnsError -v`

Expected: PASS

- [ ] **Step 5: Run the whole package to check for regressions**

Run: `go test ./cmd/rune`

Expected: ok (all tests pass, including `TestRunScript_FauxTextTurn`)

- [ ] **Step 6: Commit**

```bash
git add cmd/rune/script.go cmd/rune/script_test.go
git commit -m "fix(headless): return agent TurnError from runScript"
```

---

### Task 2: Guard the save-error-over-TurnError precedence in `runScript`

The issue mandates the order: session save error first, then TurnError. Task 1's implementation already does this; this test pins it so a future refactor can't silently swap the order. It should pass immediately.

**Files:**
- Test: `cmd/rune/script_test.go`

- [ ] **Step 1: Write the test**

Append to `cmd/rune/script_test.go`:

```go
func TestRunScript_SaveErrorTakesPrecedenceOverTurnError(t *testing.T) {
	dir := t.TempDir()
	// Point the session path at an existing directory: session.Save writes a
	// temp file then os.Rename's it onto the path, and renaming a file onto a
	// directory fails — forcing a save error.
	sessDir := filepath.Join(dir, "session-as-dir")
	if err := os.Mkdir(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sc := scriptFile{
		Provider: "faux",
		Session:  sessDir,
		Model:    "gpt-5",
		Faux: []fauxStep{
			{Reply: "partial"}, // no Done step → also triggers a TurnError
		},
		UserMessage: "hi",
	}
	b, _ := json.Marshal(sc)
	scriptPath := filepath.Join(dir, "in.json")
	_ = os.WriteFile(scriptPath, b, 0o644)

	var out bytes.Buffer
	err := runScript(context.Background(), scriptPath, &out, faux.New())
	if err == nil || !strings.Contains(err.Error(), "save session") {
		t.Fatalf("err = %v, want save session error to win over TurnError", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test ./cmd/rune -run TestRunScript_SaveErrorTakesPrecedenceOverTurnError -v`

Expected: PASS (Task 1's implementation already returns the save error before `turnErr`). If it FAILS, the return ordering in `runScript` is wrong — the `sess.Save()` check must come before `return turnErr`.

- [ ] **Step 3: Commit**

```bash
git add cmd/rune/script_test.go
git commit -m "test(headless): pin save-error precedence over TurnError in runScript"
```

---

### Task 3: Propagate TurnError from `runPrompt`

**Files:**
- Modify: `cmd/rune/prompt.go:135-163`
- Test: `cmd/rune/prompt_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/rune/prompt_test.go` (imports already cover everything needed):

```go
func TestRunPrompt_TurnErrorReturnsError(t *testing.T) {
	// HTTP 400 classifies as ai.ErrFatal in the ollama provider — not retried,
	// so the agent loop emits a single TurnError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	var buf bytes.Buffer
	err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", "", "", "", &buf)
	if err == nil {
		t.Fatalf("want non-nil error; transcript = %q", buf.String())
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("err = %v, want provider status 400 error", err)
	}
	if !strings.Contains(buf.String(), "[error:") {
		t.Fatalf("transcript missing [error: ...] line: %q", buf.String())
	}
	// The session must still be saved before the error is returned.
	sessions, _ := os.ReadDir(filepath.Join(runeDir, "sessions"))
	if len(sessions) == 0 {
		t.Fatal("session not saved before error return")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/rune -run TestRunPrompt_TurnErrorReturnsError -v`

Expected: FAIL with `want non-nil error` (current code returns nil).

- [ ] **Step 3: Implement the capture in `runPrompt`**

In `cmd/rune/prompt.go`, replace the event loop and tail of the function (currently lines 135-163):

```go
	incomplete := false
	var turnErr error
	for ev := range a.Run(ctx, msg) {
		switch v := ev.(type) {
		case agent.AssistantText:
			fmt.Fprint(w, v.Delta)
		case agent.ToolStarted:
			fmt.Fprintf(w, "\n[tool: %s]", v.Call.Name)
		case agent.ToolFinished:
			fmt.Fprintf(w, "\n[done: %d bytes]", len(v.Result.Output))
		case agent.RequiredToolPending:
			fmt.Fprintf(w, "\n[persist: must call %v before ending (attempt %d)]", v.Names, v.Attempt)
		case agent.TurnError:
			if turnErr == nil {
				turnErr = v.Err
			}
			fmt.Fprintf(w, "\n[error: %v]", v.Err)
		case agent.TurnDone:
			if v.Reason == agent.ReasonIncompleteRequiredTool {
				incomplete = true
				fmt.Fprintf(w, "\n[incomplete: ended without calling a required completion tool]")
			}
		}
	}
	fmt.Fprintln(w)
	if err := sess.Save(); err != nil {
		return err
	}
	if turnErr != nil {
		return turnErr
	}
	if incomplete {
		return agent.ErrIncompleteRequiredTool
	}
	return nil
}
```

Only three things change: the `var turnErr error` declaration, the capture inside `case agent.TurnError`, and the `if turnErr != nil { return turnErr }` block inserted between the save check and the `incomplete` check. This gives exactly the mandated order: save error → TurnError → `ErrIncompleteRequiredTool` → nil.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./cmd/rune -run TestRunPrompt_TurnErrorReturnsError -v`

Expected: PASS

- [ ] **Step 5: Run the whole package to check for regressions**

Run: `go test ./cmd/rune`

Expected: ok. Pay attention to `TestRunPrompt_CanceledContextCancelsProviderRequest` — it must still pass (cancellation emits `TurnAborted`, not `TurnError`, so `runPrompt` still returns nil there).

- [ ] **Step 6: Commit**

```bash
git add cmd/rune/prompt.go cmd/rune/prompt_test.go
git commit -m "fix(headless): return agent TurnError from runPrompt"
```

---

### Task 4: Full validation

- [ ] **Step 1: Run the issue's validation command**

Run: `go test ./cmd/rune`

Expected: ok

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`

Expected: all packages ok (this change touches only `cmd/rune`, but the suite is cheap insurance).

- [ ] **Step 3: Vet**

Run: `go vet ./cmd/rune`

Expected: no output

---

## Acceptance criteria mapping

| Criterion | Where verified |
|---|---|
| Prompt mode returns non-nil error on TurnError | Task 3, `TestRunPrompt_TurnErrorReturnsError` |
| Script mode returns non-nil error on TurnError | Task 1, `TestRunScript_TurnErrorReturnsError` |
| `[error: ...]` output remains visible | Both tests assert `[error:` in the transcript |
| Error precedence: save → TurnError → incomplete → nil | Task 2 test (save > TurnError); Task 3 code ordering (TurnError > incomplete) |
| Exit-code mapping intact | No `main.go` changes; `ErrIncompleteRequiredTool` still returned distinctly → exit 3, TurnError → generic exit 1 |
| Focused tests in `cmd/rune` | Tasks 1-3 |
