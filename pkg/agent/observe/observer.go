// Package observe provides an observability layer for the agent: prompts, responses,
// memory and session handling. Events are emitted as JSONL for analysis or LLM consumption.
package observe

import "time"

// Common fields included in all events (or set by the observer when writing).
type Common struct {
	Ts         string `json:"ts"`          // RFC3339
	AgentID    string `json:"agent_id,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	Channel    string `json:"channel,omitempty"`
	ChatID     string `json:"chat_id,omitempty"`
}

func commonNow(agentID, sessionKey, channel, chatID string) Common {
	return Common{
		Ts:         time.Now().UTC().Format(time.RFC3339),
		AgentID:    agentID,
		SessionKey: sessionKey,
		Channel:    channel,
		ChatID:     chatID,
	}
}

// TurnStartEvent is emitted at the start of a user turn (after history/summary are loaded).
type TurnStartEvent struct {
	Type           string `json:"type"` // "turn_start"
	Common         `json:",inline"`
	UserMessage    string `json:"user_message,omitempty"`
	SessionMode    string `json:"session_mode,omitempty"`    // "relevant" | "full"
	HistoryCount   int    `json:"history_count,omitempty"`
	SummaryLength  int    `json:"summary_length,omitempty"`
	RelevantLimit  int    `json:"relevant_limit,omitempty"`
	FallbackKeep   int    `json:"fallback_keep,omitempty"`
}

// MemoryUsedEvent describes what memory context was injected for this turn.
type MemoryUsedEvent struct {
	Type                 string `json:"type"` // "memory_used"
	Common               `json:",inline"`
	MemoryQuery          string `json:"memory_query,omitempty"`
	RecentDays           int    `json:"recent_days,omitempty"`
	RetrieveLimit        int    `json:"retrieve_limit,omitempty"`
	MemorySource         string `json:"memory_source,omitempty"` // "retrieve" | "full"
	MemoryContextLength  int    `json:"memory_context_length,omitempty"`
	MemoryContextPreview string `json:"memory_context_preview,omitempty"`
}

// LLMRequestEvent is emitted before each LLM call (each iteration).
type LLMRequestEvent struct {
	Type               string `json:"type"` // "llm_request"
	Common             `json:",inline"`
	Iteration          int    `json:"iteration,omitempty"`
	Model              string `json:"model,omitempty"`
	MessagesCount      int    `json:"messages_count,omitempty"`
	SystemPromptLength int    `json:"system_prompt_length,omitempty"`
	ToolsCount         int    `json:"tools_count,omitempty"`
	MessagesJSON       string `json:"messages_json,omitempty"` // when IncludeFullPrompt
}

// LLMResponseEvent is emitted after each LLM response.
type LLMResponseEvent struct {
	Type         string       `json:"type"` // "llm_response"
	Common       `json:",inline"`
	Iteration    int          `json:"iteration,omitempty"`
	ContentLength int         `json:"content_length,omitempty"`
	ContentPreview string     `json:"content_preview,omitempty"`
	ToolCalls    []ToolCallSummary `json:"tool_calls,omitempty"`
}

// ToolCallSummary is a short summary of one tool call for observation.
type ToolCallSummary struct {
	Name   string `json:"name,omitempty"`
	ArgsPreview string `json:"args_preview,omitempty"`
}

// ToolExecutedEvent is emitted after each tool execution.
type ToolExecutedEvent struct {
	Type               string `json:"type"` // "tool_executed"
	Common             `json:",inline"`
	ToolName           string `json:"tool_name,omitempty"`
	ArgsPreview        string `json:"args_preview,omitempty"`
	ResultForLLMLength int    `json:"result_for_llm_length,omitempty"`
	Error              string `json:"error,omitempty"`
}

// TurnEndEvent is emitted at the end of a turn (final response).
type TurnEndEvent struct {
	Type              string `json:"type"` // "turn_end"
	Common            `json:",inline"`
	FinalContentLength int   `json:"final_content_length,omitempty"`
	FinalContentPreview string `json:"final_content_preview,omitempty"`
	TotalIterations   int    `json:"total_iterations,omitempty"`
}

// Observer receives agent events for logging or analysis.
type Observer interface {
	OnTurnStart(TurnStartEvent)
	OnMemoryUsed(MemoryUsedEvent)
	OnLLMRequest(LLMRequestEvent)
	OnLLMResponse(LLMResponseEvent)
	OnToolExecuted(ToolExecutedEvent)
	OnTurnEnd(TurnEndEvent)
}
