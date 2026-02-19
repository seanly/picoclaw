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
    "bearer_tokens": ["sk-your-secret-token"]
  }
}
```

- **enabled**: Set to `true` to serve `POST /v1/chat/completions`. Default is `false` (endpoint is off).
- **bearer_tokens**: List of allowed Bearer tokens. Requests must include `Authorization: Bearer <token>` with one of these tokens. If `enabled` is true and `bearer_tokens` is empty, all requests to the endpoint are rejected with 401.

## Request format

- **Method**: `POST`
- **Path**: `/v1/chat/completions`
- **Headers**: `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Body**: OpenAI-style JSON, e.g.:

```json
{
  "model": "picoclaw",
  "messages": [
    { "role": "system", "content": "You are helpful." },
    { "role": "user", "content": "Hello!" }
  ]
}
```

Supported fields: `model` (optional), `messages` (required), `stream` (optional; if `true`, the server returns 501 Not Implemented for now), `user` (optional; used for session key).

## Example

```bash
curl -X POST "http://localhost:18790/v1/chat/completions" \
  -H "Authorization: Bearer sk-your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{"model":"picoclaw","messages":[{"role":"user","content":"Hello"}]}'
```

Response is standard OpenAI chat completion JSON with `choices[0].message.content` containing the agent’s reply.
