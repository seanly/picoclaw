// Package session provides turn partitioning and query-based selection of
// conversation history for multi-topic session token optimization.
package session

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// Turn represents one conversation turn: from a user message up to (but not
// including) the next user message. StartIndex/EndIndex are half-open [Start, End).
type Turn struct {
	StartIndex int
	EndIndex   int
	Text       string // concatenated Content of user/assistant/tool in this turn, for scoring
}

// PartitionTurns splits messages into turns. A turn starts at a role=user
// message and ends before the next role=user. Tool chains stay with their
// assistant message.
func PartitionTurns(messages []providers.Message) []Turn {
	var turns []Turn
	for i := 0; i < len(messages); i++ {
		if messages[i].Role != "user" {
			continue
		}
		start := i
		end := i + 1
		for end < len(messages) && messages[end].Role != "user" {
			end++
		}
		var textParts []string
		for j := start; j < end; j++ {
			if messages[j].Content != "" {
				textParts = append(textParts, messages[j].Content)
			}
		}
		turns = append(turns, Turn{
			StartIndex: start,
			EndIndex:   end,
			Text:       strings.Join(textParts, " "),
		})
		i = end - 1
	}
	return turns
}

// tokenizeForMatch extracts lowercase words (len >= 2) for scoring. Matches memory.go behavior.
func tokenizeForMatch(s string) []string {
	var words []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if len(w) >= 2 {
			words = append(words, w)
		}
	}
	return words
}

// scoreTurn scores turn text against query (same logic as memory.scoreChunk).
func scoreTurn(text, queryLower string, queryWords []string) int {
	chunkLower := strings.ToLower(text)
	score := 0
	if queryLower != "" && strings.Contains(chunkLower, queryLower) {
		score += 10
	}
	for _, w := range queryWords {
		if strings.Contains(chunkLower, w) {
			score++
		}
	}
	return score
}

// SelectRelevantTurns returns turns relevant to query: up to `limit` turns by
// score, always including the last `fallbackKeep` turns. Result is in
// original order.
func SelectRelevantTurns(turns []Turn, query string, limit, fallbackKeep int) []Turn {
	query = strings.TrimSpace(query)
	queryLower := strings.ToLower(query)
	queryWords := tokenizeForMatch(queryLower)

	if limit <= 0 {
		limit = 20
	}
	if fallbackKeep <= 0 {
		fallbackKeep = 8
	}

	// Score each turn
	type scored struct {
		turn  Turn
		score int
		idx   int
	}
	scoredTurns := make([]scored, len(turns))
	for i := range turns {
		scoredTurns[i] = scored{
			turn:  turns[i],
			score: scoreTurn(turns[i].Text, queryLower, queryWords),
			idx:   i,
		}
	}

	// Indices of last fallbackKeep turns (always include)
	lastN := fallbackKeep
	if lastN > len(turns) {
		lastN = len(turns)
	}
	lastNSet := make(map[int]bool)
	for i := len(turns) - lastN; i < len(turns); i++ {
		lastNSet[i] = true
	}

	// Sort by score desc (stable: keep original order for ties)
	for i := 0; i < len(scoredTurns); i++ {
		for j := i + 1; j < len(scoredTurns); j++ {
			if scoredTurns[j].score > scoredTurns[i].score {
				scoredTurns[i], scoredTurns[j] = scoredTurns[j], scoredTurns[i]
			}
		}
	}

	// Take top `limit` by score, but add any in lastN that weren't included
	selectedIdx := make(map[int]bool)
	count := 0
	for _, s := range scoredTurns {
		if count >= limit && selectedIdx[s.idx] {
			continue
		}
		if !selectedIdx[s.idx] {
			selectedIdx[s.idx] = true
			count++
		}
	}
	for i := range lastNSet {
		selectedIdx[i] = true
	}

	// Build result in original order
	var out []Turn
	for i := range turns {
		if selectedIdx[i] {
			out = append(out, turns[i])
		}
	}
	return out
}

// MessagesFromTurns returns the slice of messages that belong to the given
// turns, in order (turns are already in original order).
func MessagesFromTurns(messages []providers.Message, turns []Turn) []providers.Message {
	if len(turns) == 0 {
		return nil
	}
	var out []providers.Message
	for _, t := range turns {
		for j := t.StartIndex; j < t.EndIndex && j < len(messages); j++ {
			out = append(out, messages[j])
		}
	}
	return out
}

// GetRelevantHistory returns history for context: when query is non-empty and
// some turns match, returns MessagesFromTurns(SelectRelevantTurns(...));
// otherwise when fallbackKeep > 0 returns the last fallbackKeep messages,
// when fallbackKeep <= 0 returns nil (no history when no match).
func GetRelevantHistory(fullHistory []providers.Message, _ string, query string, limit, fallbackKeep int) []providers.Message {
	if limit <= 0 {
		// Caller uses full history when not enabled
		return fullHistory
	}
	noFallback := fallbackKeep <= 0

	turns := PartitionTurns(fullHistory)
	if len(turns) == 0 {
		if noFallback {
			return nil
		}
		return takeLast(fullHistory, fallbackKeep)
	}

	query = strings.TrimSpace(query)
	if query == "" {
		if noFallback {
			return nil
		}
		return takeLast(fullHistory, fallbackKeep)
	}

	effectiveFallback := fallbackKeep
	if noFallback {
		effectiveFallback = 0
	}
	selected := SelectRelevantTurns(turns, query, limit, effectiveFallback)
	if noFallback {
		queryLower := strings.ToLower(query)
		queryWords := tokenizeForMatch(queryLower)
		filtered := selected[:0]
		for _, t := range selected {
			if scoreTurn(t.Text, queryLower, queryWords) > 0 {
				filtered = append(filtered, t)
			}
		}
		selected = filtered
	}
	if len(selected) == 0 {
		if noFallback {
			return nil
		}
		return takeLast(fullHistory, fallbackKeep)
	}

	return MessagesFromTurns(fullHistory, selected)
}

func takeLast(messages []providers.Message, n int) []providers.Message {
	if n <= 0 || len(messages) == 0 {
		return messages
	}
	if n >= len(messages) {
		return messages
	}
	out := make([]providers.Message, n)
	copy(out, messages[len(messages)-n:])
	return out
}
