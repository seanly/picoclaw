# Agent observation (observability)

When enabled, the agent writes **observation events** to JSONL files for analysis and for feeding into an LLM (e.g. to debug replies or memory/session behavior).

## Configuration

In `config.json`:

```json
"observation": {
  "enabled": true,
  "path": "",
  "include_full_prompt": false
}
```

- **enabled**: Turn observation on or off. Default: false.
- **path**: Directory for JSONL output. Empty = `~/.picoclaw/observe`. Files are named `YYYY-MM-DD.jsonl` (one per day).
- **include_full_prompt**: If true, each LLM request event stores the full `messages` JSON; otherwise only lengths and previews are stored.

## Event types

Each line is a JSON object with a `type` field and common fields: `ts`, `agent_id`, `session_key`, `channel`, `chat_id`.

| type | Description |
|------|-------------|
| turn_start | Start of a user turn; includes session_mode (relevant/full), history_count, summary_length. |
| memory_used | Memory context for this turn: memory_query, recent_days, retrieve_limit, memory_source (retrieve/full), memory_context_length, memory_context_preview. |
| llm_request | Before each LLM call: iteration, model, messages_count, system_prompt_length, tools_count; optionally messages_json. |
| llm_response | After each LLM response: iteration, content_length, content_preview, tool_calls. |
| tool_executed | After each tool run: tool_name, args_preview, result_for_llm_length, error. |
| turn_end | End of turn: final_content_length, final_content_preview, total_iterations. |

## Viewing and filtering

- Tail today’s file: `tail -f ~/.picoclaw/observe/$(date +%Y-%m-%d).jsonl`
- Filter by type: `jq -c 'select(.type=="llm_response")' ~/.picoclaw/observe/*.jsonl`
- Filter by session: `jq -c 'select(.session_key=="agent:main:feishu:xxx")' ~/.picoclaw/observe/*.jsonl`

## Using with an LLM

Aggregate events by session/turn (e.g. with a script), then format as Markdown or JSON: for each turn include user_message, memory_used, session_used, and the final response. Pass that plus a question (e.g. “Why did the agent answer with X instead of Y?”) to an LLM for analysis.
