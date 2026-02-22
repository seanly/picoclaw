# Hooks

Hooks let you audit agent behavior, enforce policy, and analyze prompts and conversations. Events are emitted at fixed points in the agent lifecycle and can be written to JSONL, filtered by workspace, and used for compliance or debugging.

## Event types

| Event | When |
|-------|------|
| `before_turn` | User message received, before processing the turn |
| `after_turn` | Turn finished (final reply produced) |
| `before_llm` | Before each LLM call |
| `after_llm` | After each LLM response |
| `before_tool` | Before each tool execution |
| `after_tool` | After each tool execution |
| `on_error` | On failure (LLM error or tool error) |

Each hook receives a `Context` with fields such as `turn_id`, `session_key`, `channel`, `chat_id`, `model`, `workspace`, and event-specific data (e.g. `user_message`, `tool_name`, `llm_response_summary`). Long text and arguments are sanitized/truncated by default; full content can be enabled via prompt audit (see below).

## Enabling and disabling

- **Global**: In `config.json`, set `hooks.enabled` to `true` (default) or `false`. When `false`, no hook dispatcher is created and no hooks run.
- **Workspace**: Per-workspace policy is in `hooks.yaml` or `HOOKS.md` under the workspace. Use `enabled: false` or per-event settings to disable hooks or specific events for that workspace.
- **Prompt audit**: To log full prompts and responses for analysis, set `hooks.prompt_audit.enabled` to `true` and optionally `include_full_prompt` to `true`. Path defaults to `workspace/hooks/prompt-audit.jsonl` when `prompt_audit.path` is empty.

## Use cases and configuration

### Audit / compliance only

- `hooks.enabled: true`
- Configure workspace hooks via `HOOKS.md` or `hooks.yaml`; default audit path is workspace-based (e.g. `workspace/hooks/hook-events.jsonl`).
- Use `hooks.audit_path` in config to override the default audit path if needed.

### Optimizing system prompt / analyzing conversations

- `hooks.enabled: true`
- `hooks.prompt_audit.enabled: true`
- `hooks.prompt_audit.include_full_prompt: true` to get full `messages` JSON and full user/LLM text in the audit.
- Output is JSONL (e.g. `workspace/hooks/prompt-audit.jsonl`). Use `jq` to filter by event type, session, or agent.

### Disable hooks entirely

- `hooks.enabled: false` in `config.json`.

## Config reference

In `config.json`:

```json
"hooks": {
  "enabled": true,
  "audit_path": "",
  "prompt_audit": {
    "enabled": false,
    "path": "",
    "include_full_prompt": false
  }
}
```

- **enabled**: Master switch for hooks. Default is effectively true when `hooks` is omitted (existing behavior).
- **audit_path**: Optional override for the default hook audit path.
- **prompt_audit.enabled**: Enable the built-in prompt/conversation audit handler.
- **prompt_audit.path**: File path for prompt audit JSONL. Empty = `workspace/hooks/prompt-audit.jsonl`.
- **prompt_audit.include_full_prompt**: When true, full user message, full messages JSON (before LLM), and full LLM/final response are included in hook context and written by the prompt audit handler.

## Workspace policy

- **Location**: `HOOKS.md` and/or `hooks.yaml` inside the agent workspace.
- **Priority**: YAML overrides the natural-language policy parsed from `HOOKS.md`.
- **Behavior**: Policy controls whether hooks are enabled for that workspace and per-event settings (e.g. verbosity, capture fields). See `pkg/hookpolicy` and the hook port analysis doc for details.

For migration from the old observation feature: observation has been removed; conversation and prompt analysis are done via **hooks** and **prompt_audit**. See [hooks-port-analysis.md](hooks-port-analysis.md) for the “移植与替代说明”.
