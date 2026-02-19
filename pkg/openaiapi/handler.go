// Package openaiapi provides an OpenAI-compatible POST /v1/chat/completions HTTP handler
// that forwards requests to the picoclaw agent.
package openaiapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
)

const (
	maxBodyBytes = 1024 * 1024 // 1 MiB
)

// AgentProcessDirect is the minimal interface needed to handle a chat completion.
type AgentProcessDirect interface {
	ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
}

// Handler implements http.Handler for POST /v1/chat/completions.
type Handler struct {
	cfg        *config.Config
	agentLoop  AgentProcessDirect
}

// NewHandler returns an HTTP handler for the OpenAI-compatible chat completions endpoint.
// Caller must only register it when cfg.Gateway.OpenAIAPI.Enabled is true.
func NewHandler(cfg *config.Config, agentLoop AgentProcessDirect) *Handler {
	return &Handler{cfg: cfg, agentLoop: agentLoop}
}

// ServeHTTP handles POST /v1/chat/completions.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(openAIError("method_not_allowed", "Only POST is allowed"))
		return
	}

	apiCfg := h.cfg.Gateway.OpenAIAPI
	if len(apiCfg.BearerTokens) == 0 {
		sendUnauthorized(w)
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		sendUnauthorized(w)
		return
	}
	if !validateBearerToken(token, apiCfg.BearerTokens) {
		sendUnauthorized(w)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body chatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sendError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON body")
		return
	}

	if body.Stream {
		sendError(w, http.StatusNotImplemented, "invalid_request_error", "stream=true is not supported yet")
		return
	}

	prompt, extraSystem := buildPrompt(body.Messages)
	if prompt == "" {
		sendError(w, http.StatusBadRequest, "invalid_request_error", "Missing user message in messages")
		return
	}

	if extraSystem != "" {
		prompt = extraSystem + "\n\n" + prompt
	}

	model := body.Model
	if model == "" {
		model = "picoclaw"
	}
	sessionKey := "openai:default"
	if body.User != "" {
		sessionKey = "openai:" + body.User
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	response, err := h.agentLoop.ProcessDirect(ctx, prompt, sessionKey)
	if err != nil {
		sendError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	id := "chatcmpl-" + uuid.New().String()
	created := time.Now().Unix()

	out := chatCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []choice{
			{
				Index: 0,
				Message: message{
					Role:    "assistant",
					Content: response,
				},
				FinishReason: "stop",
			},
		},
		Usage: usage{PromptTokens: 0, CompletionTokens: 0, TotalTokens: 0},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func extractBearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(v) < len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(v[len(prefix):])
}

func validateBearerToken(token string, allowed []string) bool {
	if token == "" || len(allowed) == 0 {
		return false
	}
	tb := []byte(token)
	for _, a := range allowed {
		ab := []byte(a)
		if len(ab) == len(tb) && subtle.ConstantTimeCompare(ab, tb) == 1 {
			return true
		}
	}
	return false
}

func sendUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(openAIError("invalid_request_error", "Missing or invalid Authorization"))
}

func sendError(w http.ResponseWriter, code int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(openAIError(errType, message))
}

func openAIError(errType, message string) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]string{
			"message": message,
			"type":    errType,
		},
	}
}

type chatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	User     string    `json:"user"`
}

type message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []contentPart
	Name    string      `json:"name,omitempty"`
}

type chatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage   `json:"usage"`
}

type choice struct {
	Index         int     `json:"index"`
	Message       message `json:"message"`
	FinishReason  string  `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// buildPrompt returns the user-facing message and optional system prompt from OpenAI-format messages.
func buildPrompt(messages []message) (userMessage, systemPrompt string) {
	var systemParts []string
	var lastUserOrTool string
	var history []string

	for _, m := range messages {
		role := strings.TrimSpace(m.Role)
		content := extractTextContent(m.Content)
		content = strings.TrimSpace(content)
		if role == "" || content == "" {
			continue
		}
		switch strings.ToLower(role) {
		case "system", "developer":
			systemParts = append(systemParts, content)
			continue
		case "function":
			role = "tool"
		}
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		sender := role
		if role == "assistant" {
			sender = "Assistant"
		} else if role == "user" {
			sender = "User"
		} else if m.Name != "" {
			sender = "Tool:" + m.Name
		} else {
			sender = "Tool"
		}
		lastUserOrTool = content
		history = append(history, sender+": "+content)
	}

	if lastUserOrTool == "" {
		return "", strings.Join(systemParts, "\n\n")
	}

	if len(systemParts) > 0 {
		systemPrompt = strings.Join(systemParts, "\n\n")
	}

	if len(history) <= 1 {
		return lastUserOrTool, systemPrompt
	}
	return strings.Join(history, "\n"), systemPrompt
}

func extractTextContent(content interface{}) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	arr, ok := content.([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, p := range arr {
		part, _ := p.(map[string]interface{})
		if part == nil {
			continue
		}
		if t, _ := part["type"].(string); t == "text" {
			if text, _ := part["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
		if text, _ := part["input_text"].(string); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// Ensure Handler implements http.Handler and we use agent.AgentLoop.
var _ http.Handler = (*Handler)(nil)
var _ AgentProcessDirect = (*agent.AgentLoop)(nil)
