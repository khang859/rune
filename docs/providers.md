# Providers

rune supports multiple LLM providers. Select one with:

```bash
rune --provider codex
rune --provider groq
rune --provider ollama --model llama3.2
rune --provider runpod --model openai/gpt-oss-120b
```

or persist/select it from the TUI with `/providers` or `/settings`. Use `/model` to select a model for the active provider; for Ollama, local models are discovered from the running Ollama instance when possible.

Environment override:

- `RUNE_PROVIDER=codex|groq|ollama|runpod`

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

Ollama model names are local/user-controlled tags, so rune accepts arbitrary model IDs such as `qwen3:4b`, `qwen2.5-coder:14b`, or a custom model you created with Ollama. In the TUI, `/model` lists installed local models from Ollama's `/api/tags` endpoint when available and also offers a `custom…` entry for typing any model name.

`rune login ollama` is not required; local Ollama does not use OAuth or API keys.

### Env overrides

- `RUNE_OLLAMA_MODEL` — model default for Ollama.
- `RUNE_OLLAMA_ENDPOINT` — override the OpenAI-compatible chat completions endpoint. Defaults to `http://localhost:11434/v1/chat/completions`.

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

Set `RUNE_RUNPOD_ENDPOINT` for a private/custom Runpod Serverless vLLM endpoint. Accepted values include:

- Full chat-completions URL, e.g. `https://api.runpod.ai/v2/<endpoint-id>/openai/v1/chat/completions`
- OpenAI-compatible base URL, e.g. `https://api.runpod.ai/v2/<endpoint-id>/openai/v1`
- Endpoint ID/slug only, e.g. `<endpoint-id>` (expanded to `https://api.runpod.ai/v2/<endpoint-id>/openai/v1/chat/completions`)

Set `RUNE_RUNPOD_MODEL` to the model ID deployed by that endpoint.

### Env overrides

- `RUNE_RUNPOD_MODEL` — model default for Runpod.
- `RUNE_RUNPOD_ENDPOINT` — override the chat completions endpoint.
- `RUNE_RUNPOD_API_KEY` / `RUNPOD_API_KEY` — API key.

## Shared env

- `RUNE_DIR` — override `~/.rune`.
