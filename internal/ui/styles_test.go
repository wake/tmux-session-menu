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
		{"dim: inactive", dimStyle, "#999999"},
		{"subtle", subtleStyle, "#505050"},
		{"running: success green", statusRunningStyle, "#4EBA65"},
		{"waiting: warning yellow", statusWaitingStyle, "#FFC107"},
		{"idle: inactive", statusIdleStyle, "#999999"},
		{"error: error red", statusErrorStyle, "#FF6B80"},
		{"sessionName: bright white", sessionNameStyle, "#FFFFFF"},
		{"groupName: white bold", groupNameStyle, "#FFFFFF"},
		{"key: terracotta", keyStyle, "#D77757"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fg := tc.style.GetForeground()
			assert.Equal(t, tc.expected, fg)
		})
	}
}
