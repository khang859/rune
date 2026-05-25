# Providers

rune supports multiple LLM providers. On first run, no provider is active; `rune` still opens the TUI and the splash screen points you to `/providers` and `/settings`. Select one with:

```bash
rune --provider codex
rune --provider groq
rune --provider ollama --model llama3.2
rune --provider runpod --model openai/gpt-oss-120b
rune --provider openrouter --model anthropic/claude-sonnet-4.5
```

or persist/select it from the TUI with `/providers` or `/settings`. Use `/model` to select a model for the active provider; for Ollama, local models are discovered from the running Ollama instance when possible.

Environment override:

- `RUNE_PROVIDER=codex|groq|ollama|runpod|openrouter`

## Codex (ChatGPT Pro/Plus)

Codex uses ChatGPT OAuth and the OpenAI Responses-style Codex endpoint.

### Login

```bash
rune login codex
```

Opens your default browser to OpenAI's auth page. After approval, the local
callback at `http://localhost:1455/auth/callback` exchanges the code for an
access + refresh token. Tokens are stored in `~/.rune/auth.json` (chmod 0600).

### Models

Default: `gpt-5.5`

Available from `/model` when the active provider is Codex:

- `gpt-5.5`
- `gpt-5.4`
- `gpt-5.4-mini`
- `gpt-5.3-codex`
- `gpt-5.3-codex-spark`
- `gpt-5.2`
- `gpt-5.2-codex`
- `gpt-5.1`
- `gpt-5.1-codex-max`
- `gpt-5.1-codex-mini`

### Env overrides

- `RUNE_CODEX_MODEL` — model default for Codex.
- `RUNE_CODEX_ENDPOINT` — override the Responses URL.
- `RUNE_OAUTH_TOKEN_URL` — override the token endpoint.

## Groq

Groq uses an API key and an OpenAI-compatible Chat Completions API.

### API key

Use either environment variable:

```bash
export GROQ_API_KEY=...
# or
export RUNE_GROQ_API_KEY=...
```

You can also save/replace the key in the TUI:

1. Open `/settings`
2. Select `groq api key`
3. Paste the key

Stored keys are written to `~/.rune/secrets.json` as `groq_api_key` with chmod
`0600`. Environment variables take precedence over the stored key.

`rune login groq` is not required; Groq does not use OAuth.

### Usage

```bash
rune --provider groq --model llama-3.3-70b-versatile
```

or in interactive mode:

```text
/providers
/model
```

### Models

Default: `llama-3.3-70b-versatile`

Available from `/model` when the active provider is Groq:

- `llama-3.3-70b-versatile`
- `openai/gpt-oss-120b`
- `openai/gpt-oss-20b`
- `llama-3.1-8b-instant`
- `meta-llama/llama-4-maverick-17b-128e-instruct`
- `meta-llama/llama-4-scout-17b-16e-instruct`
- `qwen/qwen3-32b`
- `deepseek-r1-distill-llama-70b`

### Env overrides

- `RUNE_GROQ_MODEL` — model default for Groq.
- `RUNE_GROQ_ENDPOINT` — override the chat completions endpoint. Defaults to `https://api.groq.com/openai/v1/chat/completions`.
- `RUNE_GROQ_API_KEY` / `GROQ_API_KEY` — API key.

## Ollama

Ollama runs models locally. Install/pull models with Ollama first, then point rune at the local model tag.

```bash
ollama serve
ollama pull llama3.2
rune --provider ollama --model llama3.2
```

Ollama model names are local/user-controlled tags, so rune accepts arbitrary model IDs such as `qwen3:4b`, `qwen2.5-coder:14b`, or a custom model you created with Ollama. In the TUI, `/model` lists installed local models from the active Ollama profile's `/api/tags` endpoint when available and also offers a `custom…` entry for typing any model name.

`rune login ollama` is not required; local Ollama does not use OAuth or API keys. If your Ollama-compatible endpoint is behind an authenticated proxy, save an optional Ollama API key from `/settings` or use an environment variable.

### Multiple Ollama servers

Use `/settings` → `add ollama profile` to create a named Ollama server profile, then `edit active profile` to set its native `/api/chat` endpoint (or paste a legacy `/v1/chat/completions` URL — rune rewrites it transparently at request time). `/providers` shows configured profiles as separate entries such as `Ollama: Local` and `Ollama: GPU Box`, so switching servers is the same flow as switching providers. Each profile stores its own default model; `/model` updates the active profile's model.

Per-profile overrides for `ollama_num_ctx` (KV cache size) and `ollama_think` (enable thinking mode for Qwen3/DeepSeek-R1-style models) can be set in `settings.json` on the profile or at the top level. Defaults: `ollama_num_ctx: 16384`, `ollama_think: false`. Set `ollama_num_ctx` to a negative value to omit the option entirely and let the model's modelfile decide.

Set `RUNE_PROVIDER_PROFILE=<profile-id>` to select a profile from the environment. CLI flags and provider/model environment variables still take precedence over profile defaults.

### Env overrides

- `RUNE_OLLAMA_MODEL` — model default for Ollama.
- `RUNE_OLLAMA_ENDPOINT` — override the native Ollama chat endpoint. Defaults to `http://localhost:11434/api/chat`. Legacy `/v1/chat/completions` URLs are rewritten transparently.
- `RUNE_OLLAMA_API_KEY` / `OLLAMA_API_KEY` — optional bearer token for authenticated Ollama-compatible endpoints.

If a selected model is not installed, run:

```bash
ollama pull <model>
```

## Runpod

Runpod uses an API key and OpenAI-compatible Chat Completions streaming. It supports Runpod Public Endpoints by default and custom/private Runpod Serverless vLLM endpoints via an endpoint override.

### API key

Use either environment variable:

```bash
export RUNPOD_API_KEY=...
# or
export RUNE_RUNPOD_API_KEY=...
```

You can also save/replace the key in the TUI:

1. Open `/settings`
2. Select `runpod api key`
3. Paste the key

Stored keys are written to `~/.rune/secrets.json` as `runpod_api_key` with chmod
`0600`. Environment variables take precedence over the stored key.

### Usage

```bash
rune --provider runpod --model openai/gpt-oss-120b
```

or in interactive mode:

```text
/providers
/model
```

### Public endpoint models

Default: `openai/gpt-oss-120b`

Available from `/model` when the active provider is Runpod:

- `openai/gpt-oss-120b` — defaults to `https://api.runpod.ai/v2/gpt-oss-120b/openai/v1/chat/completions`
- `Qwen/Qwen3-32B-AWQ` — defaults to `https://api.runpod.ai/v2/qwen3-32b-awq/openai/v1/chat/completions`

### Custom/private endpoints

Set `RUNE_RUNPOD_ENDPOINT` for a private/custom Runpod Serverless vLLM endpoint, or save `runpod_endpoint` from `/settings`. Accepted values include:

- Full chat-completions URL, e.g. `https://api.runpod.ai/v2/<endpoint-id>/openai/v1/chat/completions`
- OpenAI-compatible base URL, e.g. `https://api.runpod.ai/v2/<endpoint-id>/openai/v1`
- Endpoint ID/slug only, e.g. `<endpoint-id>` (expanded to `https://api.runpod.ai/v2/<endpoint-id>/openai/v1/chat/completions`)

Set `RUNE_RUNPOD_MODEL` or use `/model` to select the model ID deployed by that endpoint. Environment variables take precedence over saved `/settings` values.

### Env overrides

- `RUNE_RUNPOD_MODEL` — model default for Runpod.
- `RUNE_RUNPOD_ENDPOINT` — override the chat completions endpoint.
- `RUNE_RUNPOD_API_KEY` / `RUNPOD_API_KEY` — API key.

## OpenRouter

OpenRouter uses an API key and OpenAI-compatible Chat Completions streaming. Rune treats OpenRouter as a first-class provider, so model IDs are raw OpenRouter slugs such as `anthropic/claude-sonnet-4.5` or `~openai/gpt-latest`; do not add a LiteLLM-style `openrouter/` prefix.

### API key

Use either environment variable:

```bash
export OPENROUTER_API_KEY=...
# or
export RUNE_OPENROUTER_API_KEY=...
```

You can also save/replace the key in the TUI:

1. Open `/settings`
2. Select `openrouter api key`
3. Paste the key

Stored keys are written to `~/.rune/secrets.json` as `openrouter_api_key` with chmod `0600`. Environment variables take precedence over the stored key.

### Usage

```bash
rune --provider openrouter --model anthropic/claude-sonnet-4.5
rune --prompt "hi" --provider openrouter --model ~openai/gpt-latest
```

or in interactive mode:

```text
/providers
/settings
/model
```

### Models

Default: `~openai/gpt-latest`

Available as suggestions from `/model` when the active provider is OpenRouter:

- `~openai/gpt-latest`
- `~anthropic/claude-sonnet-latest`
- `openai/gpt-4o-mini`
- `anthropic/claude-sonnet-4.5`
- `google/gemini-2.5-pro`
- `deepseek/deepseek-chat-v3.1`
- `custom…`

The OpenRouter catalog changes often. Use `custom…` to paste exact slugs from https://openrouter.ai/models; custom values are saved as `openrouter_model`.

### Endpoint override

OpenRouter defaults to `https://openrouter.ai/api/v1/chat/completions`. Set `RUNE_OPENROUTER_ENDPOINT` or save `openrouter_endpoint` from `/settings` to use a different compatible endpoint. Base URLs are accepted and `/chat/completions` is appended when needed.

### Env overrides

- `RUNE_OPENROUTER_MODEL` — model default for OpenRouter.
- `RUNE_OPENROUTER_ENDPOINT` — override the chat completions endpoint.
- `RUNE_OPENROUTER_API_KEY` / `OPENROUTER_API_KEY` — API key.

## Shared env

- `RUNE_DIR` — override `~/.rune`.
