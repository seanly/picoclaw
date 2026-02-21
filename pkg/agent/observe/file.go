package observe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const previewLen = 500

// FileObserver writes observation events as JSONL to a directory, one file per day.
type FileObserver struct {
	basePath          string
	includeFullPrompt bool
	mu                sync.Mutex
}

// NewFileObserver creates a FileObserver. basePath is the directory for JSONL files (e.g. ~/.picoclaw/observe).
// includeFullPrompt controls whether LLMRequestEvent includes full messages JSON.
func NewFileObserver(basePath string, includeFullPrompt bool) *FileObserver {
	return &FileObserver{
		basePath:          basePath,
		includeFullPrompt: includeFullPrompt,
	}
}

func (f *FileObserver) writeEvent(ev any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.basePath == "" {
		return
	}
	if err := os.MkdirAll(f.basePath, 0o755); err != nil {
		return
	}
	name := time.Now().UTC().Format("2006-01-02") + ".jsonl"
	path := filepath.Join(f.basePath, name)
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	line = append(line, '\n')

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = file.Write(line)
	_ = file.Close()
}

func truncatePreview(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = previewLen
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// OnTurnStart implements Observer.
func (f *FileObserver) OnTurnStart(ev TurnStartEvent) {
	ev.Type = "turn_start"
	f.writeEvent(ev)
}

// OnMemoryUsed implements Observer.
func (f *FileObserver) OnMemoryUsed(ev MemoryUsedEvent) {
	ev.Type = "memory_used"
	ev.MemoryContextPreview = truncatePreview(ev.MemoryContextPreview, previewLen)
	f.writeEvent(ev)
}

// OnLLMRequest implements Observer.
func (f *FileObserver) OnLLMRequest(ev LLMRequestEvent) {
	ev.Type = "llm_request"
	if !f.includeFullPrompt {
		ev.MessagesJSON = ""
	}
	f.writeEvent(ev)
}

// OnLLMResponse implements Observer.
func (f *FileObserver) OnLLMResponse(ev LLMResponseEvent) {
	ev.Type = "llm_response"
	ev.ContentPreview = truncatePreview(ev.ContentPreview, previewLen)
	f.writeEvent(ev)
}

// OnToolExecuted implements Observer.
func (f *FileObserver) OnToolExecuted(ev ToolExecutedEvent) {
	ev.Type = "tool_executed"
	ev.ArgsPreview = truncatePreview(ev.ArgsPreview, 300)
	f.writeEvent(ev)
}

// OnTurnEnd implements Observer.
func (f *FileObserver) OnTurnEnd(ev TurnEndEvent) {
	ev.Type = "turn_end"
	ev.FinalContentPreview = truncatePreview(ev.FinalContentPreview, previewLen)
	f.writeEvent(ev)
}

// IncludeFullPrompt returns whether this observer records full prompt content.
func (f *FileObserver) IncludeFullPrompt() bool {
	return f.includeFullPrompt
}
