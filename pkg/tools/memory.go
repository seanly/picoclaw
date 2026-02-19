// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package tools

import (
	"context"
	"fmt"
	"strconv"
)

// MemoryRetriever is the function type for query-based memory retrieval.
// Used by MemorySearchTool to avoid importing agent package.
type MemoryRetriever func(query string, limit int) (string, error)

// MemoryAppender is the function type for appending to memory (long-term or today).
// slot is "long_term" or "today".
type MemoryAppender func(content string, slot string) error

// MemorySearchTool lets the model search long-term memory by query.
type MemorySearchTool struct {
	retrieve MemoryRetriever
}

// NewMemorySearchTool creates a memory_search tool that uses the given retriever.
func NewMemorySearchTool(retrieve MemoryRetriever) *MemorySearchTool {
	return &MemorySearchTool{retrieve: retrieve}
}

func (t *MemorySearchTool) Name() string {
	return "memory_search"
}

func (t *MemorySearchTool) Description() string {
	return "Search long-term memory by query. Returns relevant memory chunks. Use when you need to recall specific information."
}

func (t *MemorySearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (keywords or question)",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Max number of chunks to return (default 10)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemorySearchTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if t.retrieve == nil {
		return ErrorResult("memory search not configured")
	}
	query, _ := args["query"].(string)
	if query == "" {
		return ErrorResult("query is required")
	}
	limit := 10
	if l, ok := args["limit"]; ok {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
	}
	if limit <= 0 {
		limit = 10
	}
	out, err := t.retrieve(query, limit)
	if err != nil {
		return ErrorResult(fmt.Sprintf("memory search failed: %v", err))
	}
	if out == "" {
		return NewToolResult("No relevant memory found.")
	}
	return NewToolResult(out)
}

// MemoryAppendTool lets the model append to long-term memory or today's note.
type MemoryAppendTool struct {
	appendFn MemoryAppender
}

// NewMemoryAppendTool creates a memory_append tool that uses the given appender.
func NewMemoryAppendTool(appendFn MemoryAppender) *MemoryAppendTool {
	return &MemoryAppendTool{appendFn: appendFn}
}

func (t *MemoryAppendTool) Name() string {
	return "memory_append"
}

func (t *MemoryAppendTool) Description() string {
	return "Append a note to long-term memory (MEMORY.md) or today's daily note. Use for facts, preferences, or things to remember."
}

func (t *MemoryAppendTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to append (will be formatted with a newline)",
			},
			"slot": map[string]interface{}{
				"type":        "string",
				"description": "Where to append: 'long_term' (default) or 'today'",
			},
		},
		"required": []string{"content"},
	}
}

func (t *MemoryAppendTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	if t.appendFn == nil {
		return ErrorResult("memory append not configured")
	}
	content, _ := args["content"].(string)
	if content == "" {
		return ErrorResult("content is required")
	}
	slot := "long_term"
	if s, ok := args["slot"].(string); ok && s != "" {
		slot = s
	}
	if slot != "long_term" && slot != "today" {
		return ErrorResult("slot must be 'long_term' or 'today'")
	}
	if err := t.appendFn(content, slot); err != nil {
		return ErrorResult(fmt.Sprintf("memory append failed: %v", err))
	}
	return NewToolResult("Appended to " + slot + " memory.")
}
