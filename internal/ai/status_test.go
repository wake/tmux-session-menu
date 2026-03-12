package ai_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/ai"
)

func TestDetectModel(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"sonnet model", "Using claude-sonnet-4-6\n> ", "claude-sonnet-4-6"},
		{"opus model", "Model: claude-opus-4-6\nProcessing...", "claude-opus-4-6"},
		{"haiku model", "claude-haiku-4-5-20251001 ready", "claude-haiku-4-5-20251001"},
		{"no model", "regular shell output", ""},
		{"status line model", "\u256d claude-sonnet-4-6 \u00b7 $0.02", "claude-sonnet-4-6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ai.DetectModel(tt.content))
		})
	}
}

func TestDetectTool(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"claude code prompt", "some output\n>", "claude-code"},
		{"claude code interrupt", "ctrl+c to interrupt", "claude-code"},
		{"plain shell", "user@host:~$", ""},
		{"gemini", "gemini>", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ai.DetectTool(tt.content))
		})
	}
}

func TestHasStrongAiPresence(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{"model string", "output\nclaude-sonnet-4-20250514\n> ", true},
		{"busy indicator ctrl+c", "working...\nctrl+c to interrupt\n", true},
		{"busy indicator esc", "working...\nesc to interrupt\n", true},
		{"plain shell", "user@host:~$ ls\nfile1 file2\nuser@host:~$ ", false},
		{"empty", "", false},
		{"just prompt", "> ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, ai.HasStrongAiPresence(tt.content))
		})
	}
}
