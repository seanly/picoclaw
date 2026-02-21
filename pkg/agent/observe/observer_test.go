package observe

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileObserver_WritesEvents(t *testing.T) {
	dir := t.TempDir()
	obs := NewFileObserver(dir, false)

	common := commonNow("main", "agent:main:feishu:chat1", "feishu", "chat1")

	obs.OnTurnStart(TurnStartEvent{
		Common:        common,
		UserMessage:   "hello",
		SessionMode:   "full",
		HistoryCount:  2,
		SummaryLength: 0,
	})

	obs.OnMemoryUsed(MemoryUsedEvent{
		Common:                common,
		MemoryQuery:           "hello",
		RecentDays:            3,
		RetrieveLimit:         10,
		MemorySource:          "retrieve",
		MemoryContextLength:   100,
		MemoryContextPreview:  "## Long-term Memory\n\nUser likes coffee.",
	})

	obs.OnTurnEnd(TurnEndEvent{
		Common:               common,
		FinalContentLength:   50,
		FinalContentPreview:  "Hi there!",
		TotalIterations:      1,
	})

	// Expect one file: YYYY-MM-DD.jsonl
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	path := filepath.Join(dir, entries[0].Name())
	if !strings.HasSuffix(path, ".jsonl") {
		t.Errorf("expected .jsonl file, got %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	lines := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines++
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Errorf("line %d: invalid json: %v", lines, err)
			continue
		}
		typ, _ := m["type"].(string)
		if typ == "" {
			t.Errorf("line %d: missing type", lines)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if lines != 3 {
		t.Errorf("expected 3 event lines, got %d", lines)
	}
}

func TestFileObserver_IncludeFullPromptOmitsMessagesWhenFalse(t *testing.T) {
	dir := t.TempDir()
	obs := NewFileObserver(dir, false)

	common := commonNow("main", "sess", "ch", "chat")
	obs.OnLLMRequest(LLMRequestEvent{
		Common:          common,
		Iteration:       1,
		Model:           "gpt-4",
		MessagesCount:   3,
		SystemPromptLength: 1000,
		ToolsCount:      5,
		MessagesJSON:    "[{\"role\":\"system\"}]", // should be dropped
	})

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	path := filepath.Join(dir, entries[0].Name())
	data, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["messages_json"]; ok {
		t.Error("expected messages_json to be omitted when includeFullPrompt is false")
	}
}
