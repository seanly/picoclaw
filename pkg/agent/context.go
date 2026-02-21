package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// MemoryContextOpts holds parameters for memory retrieval (recent days, retrieve limit).
// When nil, defaults are used (3 days, limit 10).
type MemoryContextOpts struct {
	RecentDays    int
	RetrieveLimit int
}

type ContextBuilder struct {
	workspace     string
	skillsLoader  *skills.SkillsLoader
	memory        *MemoryStore
	tools         *tools.ToolRegistry // Direct reference to tool registry
	memoryOpts    *MemoryContextOpts
	memoryPolicy  MemoryPolicy
}

func getGlobalConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picoclaw")
}

func NewContextBuilder(workspace string) *ContextBuilder {
	// builtin skills: skills directory in current project
	// Use the skills/ directory under the current working directory
	wd, _ := os.Getwd()
	builtinSkillsDir := filepath.Join(wd, "skills")
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	return &ContextBuilder{
		workspace:    workspace,
		skillsLoader: skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(workspace),
	}
}

// SetToolsRegistry sets the tools registry for dynamic tool summary generation.
func (cb *ContextBuilder) SetToolsRegistry(registry *tools.ToolRegistry) {
	cb.tools = registry
}

// SetMemoryOpts sets memory context options (recent days, retrieve limit). Nil uses defaults.
// Ignored when SetMemoryPolicy is used.
func (cb *ContextBuilder) SetMemoryOpts(opts *MemoryContextOpts) {
	cb.memoryOpts = opts
}

// SetMemoryPolicy sets the full memory policy. When non-nil, overrides memory opts for retrieve params.
func (cb *ContextBuilder) SetMemoryPolicy(policy MemoryPolicy) {
	cb.memoryPolicy = policy
}

// GetMemoryStore returns the memory store for backup/compress and tools.
func (cb *ContextBuilder) GetMemoryStore() *MemoryStore {
	return cb.memory
}

func (cb *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	workspacePath, _ := filepath.Abs(filepath.Join(cb.workspace))
	runtime := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Build tools section dynamically
	toolsSection := cb.buildToolsSection()

	return fmt.Sprintf(`# picoclaw 🦞

You are picoclaw, a helpful AI assistant.

## Current Time
%s

## Runtime
%s

## Workspace
Your workspace is at: %s
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md

%s

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

3. **Memory** - When something is memorable (new fact, preference, or correction), you MUST call the **memory_append** tool in the same turn. Do not only say "我会更新" or "I'll remember"—actually call memory_append. This includes when the user corrects a fact (e.g. "不是，是黑咖啡"): call memory_append with the corrected content so it is written to %s/memory/MEMORY.md. When correcting, use the same key terms as the original fact (e.g. "用户喜欢的咖啡口味：黑咖啡") so retrieval prefers the latest entry.

4. **Memory over session** - When the **Long-term Memory** section (above) and the **current session** history disagree on a user fact or preference (e.g. coffee taste, name), treat **Long-term Memory** as the source of truth. The user may have corrected it in another chat or earlier; do not repeat the outdated version from the session.`,
		now, runtime, workspacePath, workspacePath, workspacePath, workspacePath, toolsSection, workspacePath)
}

func (cb *ContextBuilder) buildToolsSection() string {
	if cb.tools == nil {
		return ""
	}

	summaries := cb.tools.GetSummaries()
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString(
		"**CRITICAL**: You MUST use tools to perform actions. Do NOT pretend to execute commands or schedule tasks.\n\n",
	)
	sb.WriteString("You have access to the following tools:\n\n")
	for _, s := range summaries {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

// BuildSystemPrompt builds the system prompt. memoryQuery is the current user message (or session summary)
// used for query-based memory retrieval; empty string uses full long-term + recent days fallback.
func (cb *ContextBuilder) BuildSystemPrompt(memoryQuery string) string {
	parts := []string{}

	// Core identity section
	parts = append(parts, cb.getIdentity())

	// Bootstrap files
	bootstrapContent := cb.LoadBootstrapFiles()
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// Skills - show summary, AI can read full content with read_file tool
	skillsSummary := cb.skillsLoader.BuildSkillsSummary()
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.

%s`, skillsSummary))
	}

	// Memory context (query-based when memoryQuery non-empty)
	recentDays, retrieveLimit := DefaultRecentDays, DefaultRetrieveLimit
	if cb.memoryPolicy != nil {
		recentDays = cb.memoryPolicy.RecentDays()
		retrieveLimit = cb.memoryPolicy.RetrieveLimit()
	} else if cb.memoryOpts != nil {
		if cb.memoryOpts.RecentDays > 0 {
			recentDays = cb.memoryOpts.RecentDays
		}
		if cb.memoryOpts.RetrieveLimit > 0 {
			retrieveLimit = cb.memoryOpts.RetrieveLimit
		}
	}
	memoryContext := cb.memory.GetMemoryContext(memoryQuery, recentDays, retrieveLimit)
	if memoryContext != "" {
		parts = append(parts, memoryContext)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
	}

	var sb strings.Builder
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			fmt.Fprintf(&sb, "## %s\n\n%s\n\n", filename, data)
		}
	}

	return sb.String()
}

func (cb *ContextBuilder) BuildMessages(
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	channel, chatID string,
) []providers.Message {
	messages := []providers.Message{}

	// Use current user message as memory query for relevant retrieval
	systemPrompt := cb.BuildSystemPrompt(currentMessage)

	// Add Current Session info if provided
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	// Log system prompt summary for debugging (debug mode only)
	logger.DebugCF("agent", "System prompt built",
		map[string]any{
			"total_chars":   len(systemPrompt),
			"total_lines":   strings.Count(systemPrompt, "\n") + 1,
			"section_count": strings.Count(systemPrompt, "\n\n---\n\n") + 1,
		})

	// Log preview of system prompt (avoid logging huge content)
	preview := systemPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "... (truncated)"
	}
	logger.DebugCF("agent", "System prompt preview",
		map[string]any{
			"preview": preview,
		})

	if summary != "" {
		systemPrompt += "\n\n## Summary of Previous Conversation\n\n" + summary
	}

	history = sanitizeHistoryForProvider(history)

	messages = append(messages, providers.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	messages = append(messages, history...)

	if strings.TrimSpace(currentMessage) != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: currentMessage,
		})
	}

	return messages
}

func sanitizeHistoryForProvider(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}

	sanitized := make([]providers.Message, 0, len(history))
	for _, msg := range history {
		switch msg.Role {
		case "tool":
			if len(sanitized) == 0 {
				logger.DebugCF("agent", "Dropping orphaned leading tool message", map[string]any{})
				continue
			}
			last := sanitized[len(sanitized)-1]
			if last.Role != "assistant" || len(last.ToolCalls) == 0 {
				logger.DebugCF("agent", "Dropping orphaned tool message", map[string]any{})
				continue
			}
			sanitized = append(sanitized, msg)

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if len(sanitized) == 0 {
					logger.DebugCF("agent", "Dropping assistant tool-call turn at history start", map[string]any{})
					continue
				}
				prev := sanitized[len(sanitized)-1]
				if prev.Role != "user" && prev.Role != "tool" {
					logger.DebugCF(
						"agent",
						"Dropping assistant tool-call turn with invalid predecessor",
						map[string]any{"prev_role": prev.Role},
					)
					continue
				}
			}
			sanitized = append(sanitized, msg)

		default:
			sanitized = append(sanitized, msg)
		}
	}

	return sanitized
}

func (cb *ContextBuilder) AddToolResult(
	messages []providers.Message,
	toolCallID, toolName, result string,
) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

func (cb *ContextBuilder) AddAssistantMessage(
	messages []providers.Message,
	content string,
	toolCalls []map[string]any,
) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// Always add assistant message, whether or not it has tool calls
	messages = append(messages, msg)
	return messages
}

func (cb *ContextBuilder) loadSkills() string {
	allSkills := cb.skillsLoader.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var skillNames []string
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}

	content := cb.skillsLoader.LoadSkillsForContext(skillNames)
	if content == "" {
		return ""
	}

	return "# Skill Definitions\n\n" + content
}

// GetSkillsInfo returns information about loaded skills.
func (cb *ContextBuilder) GetSkillsInfo() map[string]any {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]any{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
	}
}
