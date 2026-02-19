package openaiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

type mockAgent struct {
	lastPrompt   string
	lastSession  string
	response     string
	err          error
}

func (m *mockAgent) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	m.lastPrompt = content
	m.lastSession = sessionKey
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestHandler_Unauthorized(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.Enabled = true
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}

	agent := &mockAgent{response: "ok"}
	h := NewHandler(cfg, agent)

	body := []byte(`{"messages":[{"role":"user","content":"Hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No Authorization header
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong token, got %d", rec2.Code)
	}
}

func TestHandler_EmptyBearerTokens_Rejects(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.Enabled = true
	cfg.Gateway.OpenAIAPI.BearerTokens = nil

	agent := &mockAgent{response: "ok"}
	h := NewHandler(cfg, agent)

	body := []byte(`{"messages":[{"role":"user","content":"Hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer any")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when bearer_tokens empty, got %d", rec.Code)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	h := NewHandler(cfg, &mockAgent{})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rec.Code)
	}
}

func TestHandler_InvalidJSON_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	h := NewHandler(cfg, &mockAgent{})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestHandler_EmptyMessages_BadRequest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	h := NewHandler(cfg, &mockAgent{})

	body := []byte(`{"messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty messages, got %d", rec.Code)
	}
}

func TestHandler_StreamNotImplemented(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	h := NewHandler(cfg, &mockAgent{})

	body := []byte(`{"messages":[{"role":"user","content":"Hi"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 for stream=true, got %d", rec.Code)
	}
}

func TestHandler_Success_PromptAndSession(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	mock := &mockAgent{response: "Hello back"}
	h := NewHandler(cfg, mock)

	body := []byte(`{"model":"picoclaw","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hello"}],"user":"alice"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if mock.lastSession != "openai:alice" {
		t.Errorf("expected session openai:alice, got %q", mock.lastSession)
	}
	// Prompt should contain system + user message
	if mock.lastPrompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(mock.lastPrompt, "You are helpful") {
		t.Errorf("prompt should contain system content: %q", mock.lastPrompt)
	}
	if !strings.Contains(mock.lastPrompt, "Hello") {
		t.Errorf("prompt should contain user content: %q", mock.lastPrompt)
	}

	var out chatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Object != "chat.completion" || len(out.Choices) != 1 || out.Choices[0].Message.Content != "Hello back" {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestHandler_SessionKeyDefaultWhenNoUser(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAPI.BearerTokens = []string{"secret"}
	mock := &mockAgent{response: "ok"}
	h := NewHandler(cfg, mock)

	body := []byte(`{"messages":[{"role":"user","content":"Hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if mock.lastSession != "openai:default" {
		t.Errorf("expected session openai:default, got %q", mock.lastSession)
	}
}

