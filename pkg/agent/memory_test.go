// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryStore_Retrieve_emptyFile(t *testing.T) {
	dir := t.TempDir()
	ms := NewMemoryStore(dir)
	out, err := ms.Retrieve("foo", 5)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("expected empty, got %q", out)
	}
}

func TestMemoryStore_Retrieve_keywordMatch(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)
	memFile := filepath.Join(memDir, "MEMORY.md")
	content := "# Long-term Memory\n\n## User name\nUser is Alice.\n\n## Project\nWorking on project X.\n\n## Pet\nHas a dog named Bob."
	if err := os.WriteFile(memFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ms := NewMemoryStore(dir)

	out, err := ms.Retrieve("Alice", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected Alice in output, got %q", out)
	}

	out2, _ := ms.Retrieve("project X", 2)
	if !strings.Contains(out2, "project") || !strings.Contains(out2, "X") {
		t.Errorf("expected project X in output, got %q", out2)
	}
}

func TestMemoryStore_GetMemoryContext_emptyQuery_fallback(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)
	os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("## Facts\nUser likes coffee."), 0644)
	ms := NewMemoryStore(dir)

	ctx := ms.GetMemoryContext("", 3, 10)
	if !strings.Contains(ctx, "Long-term Memory") {
		t.Errorf("expected Long-term Memory section, got %q", ctx)
	}
	if !strings.Contains(ctx, "coffee") {
		t.Errorf("expected full content, got %q", ctx)
	}
}

func TestMemoryStore_GetMemoryContext_withQuery(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)
	content := "## Name\nUser is Alice.\n\n## Pet\nHas a dog."
	os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(content), 0644)
	ms := NewMemoryStore(dir)

	ctx := ms.GetMemoryContext("Alice", 2, 5)
	if !strings.Contains(ctx, "relevant") {
		t.Errorf("expected relevant section label, got %q", ctx)
	}
	if !strings.Contains(ctx, "Alice") {
		t.Errorf("expected Alice in retrieved context, got %q", ctx)
	}
}

func TestMemoryStore_GetMemoryContext_defaults(t *testing.T) {
	dir := t.TempDir()
	ms := NewMemoryStore(dir)
	ctx := ms.GetMemoryContext("", 0, 0)
	if ctx != "" {
		t.Errorf("empty store should return empty context, got %q", ctx)
	}
}

func TestMemoryStore_WriteLongTerm_backupAndAtomic(t *testing.T) {
	dir := t.TempDir()
	ms := NewMemoryStore(dir)
	// First write creates file (no backup)
	if err := ms.WriteLongTerm("first"); err != nil {
		t.Fatal(err)
	}
	if ms.ReadLongTerm() != "first" {
		t.Errorf("expected first, got %q", ms.ReadLongTerm())
	}
	// Second write should backup then replace
	if err := ms.WriteLongTerm("second"); err != nil {
		t.Fatal(err)
	}
	if ms.ReadLongTerm() != "second" {
		t.Errorf("expected second, got %q", ms.ReadLongTerm())
	}
	backups, _ := os.ReadDir(ms.backupsDir())
	if len(backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(backups))
	}
}
