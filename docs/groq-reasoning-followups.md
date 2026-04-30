# Groq reasoning — open follow-ups

Tracks known gaps in Groq reasoning support after the per-model
`reasoning_effort` gating fix landed (`internal/providers/groq.go`).

## 1. `reasoning_format` is not wired up

Groq exposes a separate parameter `reasoning_format` that controls how the
model's thought process is returned in the response. It is on a different
axis from `reasoning_effort` and has its own per-model gating.

Per Groq docs (https://console.groq.com/docs/reasoning):

- `reasoning_format` accepts `"raw"`, `"parsed"`, or `"hidden"`.
- Must be `"parsed"` or `"hidden"` when using tool calling or JSON mode.
- Supported by Qwen3 32B and DeepSeek-R1-Distill-Llama-70B.
- **Not** supported by GPT-OSS 20B / 120B — those models use a different
  knob, `include_reasoning` (boolean), and emit reasoning in a `reasoning`
  field on the assistant message instead.

Today rune does neither — it never sets `reasoning_format` or
`include_reasoning` on Groq requests. The SSE parser already reads both
`reasoning_content` and `reasoning` fields (`internal/ai/groq/sse.go:67`),
so reasoning streams that the model emits *by default* still surface, but
we don't control format or visibility.

What this means in practice:

- On Qwen3 / DeepSeek-R1 with tool calling, Groq's default `raw` format may
  cause issues (docs say `parsed` or `hidden` is required when tools are
  used). We have not seen a failure yet but the contract says it's wrong.
- On GPT-OSS, we can't toggle whether reasoning is included — we always get
  whatever the default is.

### Suggested fix

Extend the capability table in `internal/providers/groq.go` with a second
function, e.g. `GroqReasoningFormat(model string) string`, that returns:

- `"parsed"` for Qwen3 and DeepSeek-R1 (always safe, works with tools).
- `""` for GPT-OSS — and instead set `include_reasoning: true` on the wire.
- `""` for everything else (omit the field).

Add the corresponding wire fields to `payload` in
`internal/ai/groq/request.go`:

```go
ReasoningFormat   string `json:"reasoning_format,omitempty"`
IncludeReasoning  *bool  `json:"include_reasoning,omitempty"`
```

Wire them up in `buildPayload` based on the new capability function. Add
unit tests covering each model's expected wire shape.

## 2. UI levels for Qwen3 are pretend

`thinkingLevelsForModel` exposes `none / low / medium / high` for Qwen3,
but Groq only accepts `none / default` for that model. The request layer
remaps any non-`none` choice to `"default"`
(`internal/providers/groq.go:GroqReasoningEffort`), so the user can't
actually distinguish low/medium/high on Qwen3 — they all behave the same.

This is intentional for now (preserves consistent UX across Groq models
without UI surgery), but means the picker is misleading on Qwen3. Options:

- **(a)** Leave as-is, document it.
- **(b)** Change `GroqThinkingLevels("qwen/qwen3-32b")` to return
  `["none", "default"]` and update the picker rendering to handle that
  set. More accurate, but adds a model-specific UI branch.

## 3. Live model discovery

The current Groq model list in `internal/providers/providers.go:GroqModels`
is hardcoded. Groq exposes `GET /openai/v1/models` which would let us
discover new models (and detect deprecations) without a code change. Not
urgent, but worth considering once the model lineup churns again — Groq
adds/removes models more frequently than Codex.
