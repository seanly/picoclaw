package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sipeed/picoclaw/pkg/hooks"
)

// PromptAuditHandler writes hook events to a JSONL file for conversation/prompt analysis (e.g. system prompt optimization).
// When Context contains full-content fields (MessagesJSON, FullUserMessage, FullLLMResponseSummary), they are written as-is.
type PromptAuditHandler struct {
	path string
	mu   sync.Mutex
}

// NewPromptAuditHandler creates a handler that appends to the given path (e.g. workspace/hooks/prompt-audit.jsonl).
func NewPromptAuditHandler(path string) (*PromptAuditHandler, error) {
	if path == "" {
		return nil, fmt.Errorf("prompt audit path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create prompt audit dir: %w", err)
	}
	return &PromptAuditHandler{path: path}, nil
}

func (h *PromptAuditHandler) Name() string {
	return "prompt_audit"
}

// promptAuditEntry is the JSONL line shape for analysis (session, turn, event, and optional full content).
type promptAuditEntry struct {
	Type                   string         `json:"type"`                              // hook event name, e.g. before_turn, before_llm
	Ts                     string         `json:"ts"`                                // RFC3339
	TurnID                 string         `json:"turn_id,omitempty"`
	SessionKey             string         `json:"session_key,omitempty"`
	Channel                string         `json:"channel,omitempty"`
	ChatID                 string         `json:"chat_id,omitempty"`
	AgentID                string         `json:"agent_id,omitempty"` // from Metadata if set
	Model                  string         `json:"model,omitempty"`
	UserMessage            string         `json:"user_message,omitempty"`             // truncated
	FullUserMessage        string         `json:"full_user_message,omitempty"`         // when include_full_prompt
	MessagesJSON           string         `json:"messages_json,omitempty"`             // when include_full_prompt (EventBeforeLLM)
	LLMResponseSummary     string         `json:"llm_response_summary,omitempty"`     // truncated
	FullLLMResponseSummary string         `json:"full_llm_response_summary,omitempty"` // when include_full_prompt
	ToolName               string         `json:"tool_name,omitempty"`
	ErrorMessage           string         `json:"error_message,omitempty"`
	Metadata               map[string]any  `json:"metadata,omitempty"`
}

func (h *PromptAuditHandler) Handle(_ context.Context, ev hooks.Event, data hooks.Context) hooks.Result {
	entry := promptAuditEntry{
		Type:               string(ev),
		Ts:                 data.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		TurnID:             data.TurnID,
		SessionKey:         data.SessionKey,
		Channel:            data.Channel,
		ChatID:             data.ChatID,
		Model:              data.Model,
		UserMessage:        data.UserMessage,
		FullUserMessage:    data.FullUserMessage,
		MessagesJSON:       data.MessagesJSON,
		LLMResponseSummary: data.LLMResponseSummary,
		FullLLMResponseSummary: data.FullLLMResponseSummary,
		ToolName:           data.ToolName,
		ErrorMessage:       data.ErrorMessage,
		Metadata:           data.Metadata,
	}
	if data.Metadata != nil {
		if id, _ := data.Metadata["agent_id"].(string); id != "" {
			entry.AgentID = id
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	b, err := json.Marshal(entry)
	if err != nil {
		return hooks.Result{Status: hooks.StatusError, Message: "prompt_audit marshal failed", Err: err}
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return hooks.Result{Status: hooks.StatusError, Message: "prompt_audit open failed", Err: err}
	}
	_, err = f.Write(append(b, '\n'))
	f.Close()
	if err != nil {
		return hooks.Result{Status: hooks.StatusError, Message: "prompt_audit write failed", Err: err}
	}
	return hooks.Result{Status: hooks.StatusOK, Message: "prompt_audit written"}
}
