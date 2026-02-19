// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MemoryStore manages persistent memory for the agent.
// - Long-term memory: memory/MEMORY.md
// - Daily notes: memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	workspace  string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore creates a new MemoryStore with the given workspace path.
// It ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")

	// Ensure memory directory exists
	os.MkdirAll(memoryDir, 0755)

	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
	}
}

// getTodayFile returns the path to today's daily note file (memory/YYYYMM/YYYYMMDD.md).
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// backupsDir returns memory/backups for write-after-backup and compress.
func (ms *MemoryStore) backupsDir() string {
	return filepath.Join(ms.memoryDir, "backups")
}

// policySnapshotsDir returns memory/policy_snapshots for strategy rollback.
func (ms *MemoryStore) policySnapshotsDir() string {
	return filepath.Join(ms.memoryDir, "policy_snapshots")
}

// SavePolicySnapshot writes a policy config snapshot to memory/policy_snapshots/YYYYMMDD_HHMMSS.json.
// Call before applying policy updates so rollback is possible.
func (ms *MemoryStore) SavePolicySnapshot(configJSON []byte) error {
	dir := ms.policySnapshotsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	name := time.Now().Format("20060102_150405") + ".json"
	return os.WriteFile(filepath.Join(dir, name), configJSON, 0644)
}

// BackupLongTerm copies current MEMORY.md to memory/backups/YYYYMMDD_HHMMSS_MEMORY.md.
// No-op if MEMORY.md does not exist. Call before overwrite or compress.
func (ms *MemoryStore) BackupLongTerm() error {
	data, err := os.ReadFile(ms.memoryFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	backDir := ms.backupsDir()
	if err := os.MkdirAll(backDir, 0755); err != nil {
		return err
	}
	name := time.Now().Format("20060102_150405") + "_MEMORY.md"
	dest := filepath.Join(backDir, name)
	return os.WriteFile(dest, data, 0644)
}

// WriteLongTerm writes content to the long-term memory file (MEMORY.md).
// Backs up existing file first, then writes atomically (temp file + rename).
func (ms *MemoryStore) WriteLongTerm(content string) error {
	if _, err := os.Stat(ms.memoryFile); err == nil {
		if err := ms.BackupLongTerm(); err != nil {
			return fmt.Errorf("backup before write: %w", err)
		}
	}
	tmpFile := ms.memoryFile + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, ms.memoryFile); err != nil {
		os.Remove(tmpFile)
		return err
	}
	return nil
}

// ReadLongTerm reads the long-term memory (MEMORY.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// NormalizeLongTermEntry formats a single memory entry so retrieval can split it as one chunk.
// Uses "## YYYY-MM-DD" + content so splitMemoryChunks (by "\n## ") gets clear boundaries.
func NormalizeLongTermEntry(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	return "## " + time.Now().Format("2006-01-02") + "\n\n" + s
}

// ReadToday reads today's daily note.
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday appends content to today's daily note.
// If the file doesn't exist, it creates a new file with a date header.
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// Ensure month directory exists
	monthDir := filepath.Dir(todayFile)
	os.MkdirAll(monthDir, 0755)

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// Add header for new day
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// Append to existing content
		newContent = existingContent + "\n" + content
	}

	return os.WriteFile(todayFile, []byte(newContent), 0644)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var notes []string

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			notes = append(notes, string(data))
		}
	}

	if len(notes) == 0 {
		return ""
	}

	// Join with separator
	var result string
	for i, note := range notes {
		if i > 0 {
			result += "\n\n---\n\n"
		}
		result += note
	}
	return result
}

// memoryChunk is a scored segment of memory content.
type memoryChunk struct {
	text  string
	score int
}

// Retrieve returns memory content relevant to the query (lightweight: keyword/paragraph match).
// Splits MEMORY.md by ## or double newline, scores chunks by keyword overlap, returns top limit.
// If limit <= 0, defaults to 10.
func (ms *MemoryStore) Retrieve(query string, limit int) (string, error) {
	if limit <= 0 {
		limit = 10
	}
	query = strings.TrimSpace(query)
	longTerm := ms.ReadLongTerm()
	if longTerm == "" {
		return "", nil
	}
	// Split by ## headings or \n\n paragraphs
	rawChunks := splitMemoryChunks(longTerm)
	if len(rawChunks) == 0 {
		return "", nil
	}
	queryLower := strings.ToLower(query)
	queryWords := tokenizeForMatch(queryLower)
	var chunks []memoryChunk
	for _, text := range rawChunks {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		score := scoreChunk(text, queryLower, queryWords)
		chunks = append(chunks, memoryChunk{text: text, score: score})
	}
	// Sort by score desc (simple bubble for small n)
	for i := 0; i < len(chunks); i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunks[j].score > chunks[i].score {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}
	// Take top limit
	if limit > len(chunks) {
		limit = len(chunks)
	}
	var b strings.Builder
	for i := 0; i < limit && i < len(chunks); i++ {
		if chunks[i].score <= 0 && i > 0 {
			break
		}
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(chunks[i].text)
	}
	return b.String(), nil
}

func splitMemoryChunks(content string) []string {
	var chunks []string
	// Prefer ## as segment boundary
	for _, block := range strings.Split(content, "\n## ") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		// If block is large, sub-split by \n\n
		for _, para := range strings.Split(block, "\n\n") {
			para = strings.TrimSpace(para)
			if para != "" {
				chunks = append(chunks, para)
			}
		}
	}
	if len(chunks) == 0 && strings.TrimSpace(content) != "" {
		chunks = []string{content}
	}
	return chunks
}

func tokenizeForMatch(s string) []string {
	var words []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if len(w) >= 2 {
			words = append(words, w)
		}
	}
	return words
}

func scoreChunk(chunkText, queryLower string, queryWords []string) int {
	chunkLower := strings.ToLower(chunkText)
	score := 0
	if strings.Contains(chunkLower, queryLower) {
		score += 10
	}
	for _, w := range queryWords {
		if strings.Contains(chunkLower, w) {
			score++
		}
	}
	return score
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// If query is non-empty, uses Retrieve(query, retrieveLimit) and appends recent daily notes (recentDays).
// If query is empty, uses full long-term memory + recent daily notes (fallback behavior).
// recentDays and retrieveLimit use defaults 3 and 10 when <= 0.
func (ms *MemoryStore) GetMemoryContext(query string, recentDays, retrieveLimit int) string {
	if recentDays <= 0 {
		recentDays = 3
	}
	if retrieveLimit <= 0 {
		retrieveLimit = 10
	}

	var parts []string

	if query != "" {
		retrieved, _ := ms.Retrieve(query, retrieveLimit)
		if retrieved != "" {
			parts = append(parts, "## Long-term Memory (relevant)\n\n"+retrieved)
		}
	} else {
		longTerm := ms.ReadLongTerm()
		if longTerm != "" {
			parts = append(parts, "## Long-term Memory\n\n"+longTerm)
		}
	}

	recentNotes := ms.GetRecentDailyNotes(recentDays)
	if recentNotes != "" {
		parts = append(parts, "## Recent Daily Notes\n\n"+recentNotes)
	}

	if len(parts) == 0 {
		return ""
	}

	var result string
	for i, part := range parts {
		if i > 0 {
			result += "\n\n---\n\n"
		}
		result += part
	}
	return fmt.Sprintf("# Memory\n\n%s", result)
}
