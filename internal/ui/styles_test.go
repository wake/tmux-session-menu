package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestStyles_ClaudeCodeDarkMode(t *testing.T) {
	tests := []struct {
		name     string
		style    lipgloss.Style
		expected lipgloss.Color
	}{
		{"header: Claude terracotta", headerStyle, "#D77757"},
		{"selected: periwinkle", selectedStyle, "#B1B9F9"},
		{"dim: #888888", dimStyle, "#888888"},
		{"running: #91E200", statusRunningStyle, "#91E200"},
		{"waiting: #E8D969", statusWaitingStyle, "#E8D969"},
		{"idle: #AAAAAA", statusIdleStyle, "#AAAAAA"},
		{"error: error red", statusErrorStyle, "#FF6B80"},
		{"sessionName: bright white", sessionNameStyle, "#FFFFFF"},
		{"groupName: #F1E174", groupNameStyle, "#F1E174"},
		{"key: terracotta", keyStyle, "#D77757"},
		{"version: #C1C1C1", versionStyle, "#C1C1C1"},
		{"summary: #ABB5F7", summaryStyle, "#ABB5F7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fg := tc.style.GetForeground()
			assert.Equal(t, tc.expected, fg)
		})
	}
}

func TestStyles_KeyBold(t *testing.T) {
	assert.True(t, keyStyle.GetBold(), "keyStyle should be bold")
}

func TestStyles_CursorBackground(t *testing.T) {
	bg := cursorBgStyle.GetBackground()
	assert.Equal(t, lipgloss.Color("#373737"), bg, "cursor bg should be #373737")
}
