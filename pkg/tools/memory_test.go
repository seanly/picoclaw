// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package tools

import (
	"context"
	"testing"
)

func TestMemorySearchTool_Execute(t *testing.T) {
	called := false
	tool := NewMemorySearchTool(func(query string, limit int) (string, error) {
		called = true
		if query != "test" || limit != 5 {
			return "", nil
		}
		return "found", nil
	})
	res := tool.Execute(context.Background(), map[string]interface{}{
		"query": "test",
		"limit": 5,
	})
	if !called {
		t.Fatal("retriever not called")
	}
	if res.IsError {
		t.Fatal("expected success")
	}
	if res.ForLLM != "found" {
		t.Errorf("expected ForLLM=found, got %q", res.ForLLM)
	}
}

func TestMemorySearchTool_Execute_missingQuery(t *testing.T) {
	tool := NewMemorySearchTool(func(string, int) (string, error) { return "", nil })
	res := tool.Execute(context.Background(), map[string]interface{}{})
	if !res.IsError {
		t.Fatal("expected error when query missing")
	}
}

func TestMemoryAppendTool_Execute(t *testing.T) {
	var gotContent, gotSlot string
	tool := NewMemoryAppendTool(func(content, slot string) error {
		gotContent = content
		gotSlot = slot
		return nil
	})
	res := tool.Execute(context.Background(), map[string]interface{}{
		"content": "hello",
		"slot":    "today",
	})
	if res.IsError {
		t.Fatal(res.ForLLM)
	}
	if gotContent != "hello" || gotSlot != "today" {
		t.Errorf("got content=%q slot=%q", gotContent, gotSlot)
	}
}

func TestMemoryAppendTool_Execute_defaultSlot(t *testing.T) {
	var gotSlot string
	tool := NewMemoryAppendTool(func(_, slot string) error {
		gotSlot = slot
		return nil
	})
	_ = tool.Execute(context.Background(), map[string]interface{}{"content": "x"})
	if gotSlot != "long_term" {
		t.Errorf("expected slot long_term, got %q", gotSlot)
	}
}
