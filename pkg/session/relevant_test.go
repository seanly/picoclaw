package session

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestPartitionTurns(t *testing.T) {
	// Empty
	got := PartitionTurns(nil)
	if len(got) != 0 {
		t.Errorf("PartitionTurns(nil) len = %d, want 0", len(got))
	}
	got = PartitionTurns([]providers.Message{})
	if len(got) != 0 {
		t.Errorf("PartitionTurns([]) len = %d, want 0", len(got))
	}

	// No user message
	got = PartitionTurns([]providers.Message{
		{Role: "assistant", Content: "hi"},
	})
	if len(got) != 0 {
		t.Errorf("no user message: len = %d, want 0", len(got))
	}

	// One turn: user + assistant
	msgs := []providers.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	got = PartitionTurns(msgs)
	if len(got) != 1 {
		t.Fatalf("one turn: len = %d, want 1", len(got))
	}
	if got[0].StartIndex != 0 || got[0].EndIndex != 2 {
		t.Errorf("turn 0: StartIndex=%d EndIndex=%d, want 0 2", got[0].StartIndex, got[0].EndIndex)
	}
	if got[0].Text != "hello hi there" {
		t.Errorf("turn 0 Text = %q, want %q", got[0].Text, "hello hi there")
	}

	// Two turns; first turn has user, assistant, tool, assistant (tool chain not split)
	msgs = []providers.Message{
		{Role: "user", Content: "run ls"},
		{Role: "assistant", Content: "running"},
		{Role: "tool", Content: "file1.txt", ToolCallID: "1"},
		{Role: "assistant", Content: "Here are the files."},
		{Role: "user", Content: "what about the weather"},
		{Role: "assistant", Content: "I don't have weather."},
	}
	got = PartitionTurns(msgs)
	if len(got) != 2 {
		t.Fatalf("two turns: len = %d, want 2", len(got))
	}
	// Turn 0: indices 0-4 (user, assistant, tool, assistant)
	if got[0].StartIndex != 0 || got[0].EndIndex != 4 {
		t.Errorf("turn 0: StartIndex=%d EndIndex=%d, want 0 4", got[0].StartIndex, got[0].EndIndex)
	}
	// Turn 1: indices 4-6
	if got[1].StartIndex != 4 || got[1].EndIndex != 6 {
		t.Errorf("turn 1: StartIndex=%d EndIndex=%d, want 4 6", got[1].StartIndex, got[1].EndIndex)
	}
}

func TestSelectRelevantTurns(t *testing.T) {
	turns := []Turn{
		{StartIndex: 0, EndIndex: 2, Text: "weather is sunny today"},
		{StartIndex: 2, EndIndex: 4, Text: "code review feedback"},
		{StartIndex: 4, EndIndex: 6, Text: "tomorrow weather rain"},
		{StartIndex: 6, EndIndex: 8, Text: "schedule meeting"},
	}
	// Query "weather" -> turns 0 and 2 score; last 2 turns (indices 2,3) must be included
	got := SelectRelevantTurns(turns, "weather", 2, 2)
	// Should include turn at index 0 (weather), index 2 (weather), and last 2 (indices 2, 3)
	if len(got) < 2 {
		t.Errorf("SelectRelevantTurns: len = %d, want at least 2", len(got))
	}
	// Result must be in original order
	for i := 1; i < len(got); i++ {
		if got[i].StartIndex < got[i-1].StartIndex {
			t.Errorf("turns not in order: %v", got)
		}
	}
	// Last turn (schedule) must be in result (fallbackKeep)
	foundLast := false
	for _, tr := range got {
		if tr.StartIndex == 6 {
			foundLast = true
			break
		}
	}
	if !foundLast {
		t.Errorf("last turn (StartIndex 6) not in result")
	}
}

func TestMessagesFromTurns(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
	}
	turns := []Turn{
		{StartIndex: 0, EndIndex: 2, Text: ""},
		{StartIndex: 2, EndIndex: 4, Text: ""},
	}
	got := MessagesFromTurns(msgs, turns)
	if len(got) != 4 {
		t.Errorf("MessagesFromTurns len = %d, want 4", len(got))
	}
	if len(got) >= 4 && (got[0].Content != "a" || got[2].Content != "c") {
		t.Errorf("MessagesFromTurns wrong order or content: %v", got)
	}
}

func TestGetRelevantHistory_EmptyQuery(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", Content: "y"},
		{Role: "user", Content: "z"},
		{Role: "assistant", Content: "w"},
	}
	got := GetRelevantHistory(msgs, "", "", 20, 2)
	if len(got) != 2 {
		t.Errorf("empty query fallback: len = %d, want 2 (last 2 msgs)", len(got))
	}
	if len(got) >= 2 && (got[0].Content != "z" || got[1].Content != "w") {
		t.Errorf("fallback should be last 2: %v", got)
	}
}

func TestGetRelevantHistory_QueryMatch(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "tell me about dogs"},
		{Role: "assistant", Content: "Dogs are pets."},
		{Role: "user", Content: "and cats"},
		{Role: "assistant", Content: "Cats too."},
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "Sunny."},
	}
	// Query "dogs" should pull first turn; fallbackKeep 1 keeps last turn
	got := GetRelevantHistory(msgs, "", "dogs", 5, 1)
	if len(got) == 0 {
		t.Fatal("GetRelevantHistory returned empty")
	}
	// Should contain first turn (dogs) and last turn (weather)
	hasDogs := false
	hasWeather := false
	for _, m := range got {
		if m.Content == "tell me about dogs" || m.Content == "Dogs are pets." {
			hasDogs = true
		}
		if m.Content == "weather?" || m.Content == "Sunny." {
			hasWeather = true
		}
	}
	if !hasDogs {
		t.Errorf("expected relevant turn (dogs) in result")
	}
	if !hasWeather {
		t.Errorf("expected last turn (weather) in result (fallbackKeep)")
	}
}

func TestGetRelevantHistory_LimitZeroReturnsFull(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
	}
	got := GetRelevantHistory(msgs, "", "a", 0, 8)
	if len(got) != 2 {
		t.Errorf("limit 0: len = %d, want 2 (full history)", len(got))
	}
}

func TestGetRelevantHistory_NoFallbackWhenFallbackKeepZero(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "only about weather"},
		{Role: "assistant", Content: "sunny"},
	}
	// Query that does not match; fallbackKeep 0 -> expect empty
	got := GetRelevantHistory(msgs, "", "xyznonexistent", 5, 0)
	if len(got) != 0 {
		t.Errorf("no match and fallbackKeep=0: len = %d, want 0", len(got))
	}
	// Same but fallbackKeep 2 -> expect last 2 messages
	got2 := GetRelevantHistory(msgs, "", "xyznonexistent", 5, 2)
	if len(got2) != 2 {
		t.Errorf("no match and fallbackKeep=2: len = %d, want 2", len(got2))
	}
}
