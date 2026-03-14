package tmux_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func TestSession_StatusIcon_AiPresent(t *testing.T) {
	tests := []struct {
		name   string
		sess   tmux.Session
		expect string
	}{
		{"ai running", tmux.Session{AiType: "claude", Status: tmux.StatusRunning}, "●"},
		{"ai waiting", tmux.Session{AiType: "claude", Status: tmux.StatusWaiting}, "◐"},
		{"ai idle", tmux.Session{AiType: "claude", Status: tmux.StatusIdle}, "●"},
		{"ai error", tmux.Session{AiType: "claude", Status: tmux.StatusError}, "✗"},
		{"no ai running", tmux.Session{AiType: "", Status: tmux.StatusRunning}, "❯"},
		{"no ai idle", tmux.Session{AiType: "", Status: tmux.StatusIdle}, "○"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.sess.StatusIcon())
		})
	}
}
