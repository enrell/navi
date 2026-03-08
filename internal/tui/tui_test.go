package tui

import (
	"strings"
	"testing"

	agentsvc "navi/internal/core/services/agent"
)

func TestBuildPrompt_WithoutHistory(t *testing.T) {
	got := buildPrompt("hello", "", nil)
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestBuildPrompt_IncludesSummaryAndRecentTurns(t *testing.T) {
	got := buildPrompt("ship it", "Earlier summary", []conversationTurn{{User: "u1", Assistant: "a1"}, {User: "u2", Assistant: "a2"}})
	for _, want := range []string{"Conversation summary:", "Earlier summary", "Recent conversation:", "User: u1", "Assistant: a2", "Current request:", "ship it"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt %q should contain %q", got, want)
		}
	}
}

func TestShouldRenderMarkdown_PlainTextDoesNotTriggerRenderer(t *testing.T) {
	if shouldRenderMarkdown("Hello! How can I help you today?") {
		t.Fatal("plain text should not trigger markdown rendering")
	}
	if !shouldRenderMarkdown("- item one\n- item two") {
		t.Fatal("list markdown should trigger markdown rendering")
	}
}

func TestTargetTextareaHeight_ClampsBetweenOneAndFiveLines(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "empty", value: "", want: 1},
		{name: "single line", value: "hello", want: 1},
		{name: "three lines", value: "a\nb\nc", want: 3},
		{name: "over max", value: "1\n2\n3\n4\n5\n6", want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := targetTextareaHeight(tt.value); got != tt.want {
				t.Fatalf("height = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExplicitLineCount(t *testing.T) {
	if got := explicitLineCount("one\ntwo\nthree"); got != 3 {
		t.Fatalf("line count = %d, want 3", got)
	}
}

func TestTrimTextareaView_RemovesOnlyTrailingPadding(t *testing.T) {
	view := "first line\nsecond line\n   \n   \n"
	got := trimTextareaView(view, 2)
	if got != "first line\nsecond line" {
		t.Fatalf("got %q", got)
	}

	withTrailingBlankLine := "first line\n\n   \n   \n"
	got = trimTextareaView(withTrailingBlankLine, 2)
	if got != "first line\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCompactTurns_ReducesOlderTurns(t *testing.T) {
	turns := []conversationTurn{
		{User: strings.Repeat("a", 400), Assistant: strings.Repeat("b", 400)},
		{User: strings.Repeat("c", 400), Assistant: strings.Repeat("d", 400)},
		{User: strings.Repeat("e", 400), Assistant: strings.Repeat("f", 400)},
		{User: strings.Repeat("g", 400), Assistant: strings.Repeat("h", 400)},
		{User: strings.Repeat("i", 400), Assistant: strings.Repeat("j", 400)},
	}
	summary, retained, notice := compactTurns("", turns, 500)
	if summary == "" {
		t.Fatal("expected summary to be created")
	}
	if len(retained) != 4 {
		t.Fatalf("retained len = %d, want 4", len(retained))
	}
	if notice == "" {
		t.Fatal("expected compaction notice")
	}
}

func TestReplaceMentionAtCursor(t *testing.T) {
	got := replaceMentionAtCursor("check @cmd/na", 0, len([]rune("check @cmd/na")), "cmd/navi/main.go")
	if got != "check @cmd/navi/main.go" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchWorkspaceFiles_PrefersPrefix(t *testing.T) {
	files := []string{"cmd/navi/main.go", "internal/tui/tui.go", "docs/index.md"}
	got := matchWorkspaceFiles(files, "cmd", 5)
	if len(got) == 0 || got[0] != "cmd/navi/main.go" {
		t.Fatalf("matches = %+v, want cmd/navi/main.go first", got)
	}
}

func TestLoadAgentsCmd_TypedNilAgentServiceDoesNotPanic(t *testing.T) {
	var svc *agentsvc.Service
	msg := loadAgentsCmd(svc)()
	loaded, ok := msg.(agentListMsg)
	if !ok {
		t.Fatalf("msg type = %T, want agentListMsg", msg)
	}
	if loaded.Err != nil {
		t.Fatalf("unexpected error: %v", loaded.Err)
	}
	if len(loaded.Agents) != 0 {
		t.Fatalf("agents = %+v, want none", loaded.Agents)
	}
}
