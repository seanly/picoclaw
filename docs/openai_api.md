# OpenAI-compatible API

The gateway can expose an OpenAI-compatible `POST /v1/chat/completions` endpoint so that clients (e.g. OpenAI SDK, LiteLLM) can send chat requests to PicoClaw as if it were an OpenAI API server.

## Enable and configure

In your config file (e.g. `~/.picoclaw/config.json`), under `gateway`, set `openai_api`:

```json
"gateway": {
  "host": "0.0.0.0",
  "port": 18790,
  "openai_api": {
    "enabled": true,
    "bearer_tokens": ["sk-your-secret-token"],
    "model_scope": "global"
  }
}
```

- **enabled**: Set to `true` to serve `POST /v1/chat/completions`. Default is `false` (endpoint is off).
- **bearer_tokens**: List of allowed Bearer tokens. Requests must include `Authorization: Bearer <token>` with one of these tokens. If `enabled` is true and `bearer_tokens` is empty, all requests to the endpoint are rejected with 401.
- **model_scope**: How the request `model` is applied. `"global"` (default): one stored model for all requests; any request with `model` updates it, and requests without `model` use this global value (like `/switch model`). `"session"`: store model per session (`openai:default` or `openai:<user>`); later requests in the same session without `model` use the stored one.

## Request format

- **Method**: `POST`
- **Path**: `/v1/chat/completions`
- **Headers**: `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Body**: OpenAI-style JSON, e.g.:

```json
{
  "model": "qwen3-coder-plus",
  "messages": [
    { "role": "system", "content": "You are helpful." },
    { "role": "user", "content": "Hello!" }
  ]
}
```

Supported fields: `model` (optional; use a **model_name** from your config’s `model_list`, e.g. `qwen3-coder-plus`), `messages` (required), `stream` (optional; if `true`, the server returns 501 Not Implemented for now), `user` (optional; used for session key).

**Model used**: Depends on **model_scope** in config. With `model_scope: "global"` (default), one model is shared for all requests: any request with `model` updates it, and requests without `model` use this global value (like `/switch model`). With `model_scope: "session"`, the first request in a session that includes a `model` sets that model for that session; later requests in the same session without `model` use the stored one. If no model is stored and the request omits `model`, the default agent is used. The response always echoes the model used for that request. The same **model_name** (from `model_list`) selection is used by the CLI: `picoclaw agent --model <model_name>` uses that model for the run without changing config.

## Example

```bash
curl -X POST "http://localhost:18790/v1/chat/completions" \
  -H "Authorization: Bearer sk-your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"model":"qwen3-coder-plus","messages":[{"role":"user","content":"Hello"}]}'
```

Response is standard OpenAI chat completion JSON with `choices[0].message.content` containing the agent’s reply.
