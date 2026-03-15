package tmux_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func timeNowUnix() int64 { return time.Now().Unix() }

func TestResolveStatus_HookTakesPriority(t *testing.T) {
	input := tmux.StatusInput{
		HookStatus:  &tmux.HookStatus{Status: tmux.StatusRunning, Timestamp: timeNowUnix(), RawStatus: "running"},
		PaneTitle:   "",
		PaneContent: ">",
	}
	assert.Equal(t, tmux.StatusRunning, tmux.ResolveStatus(input))
}

func TestResolveStatus_TitleFallback(t *testing.T) {
	input := tmux.StatusInput{
		HookStatus:  nil,
		PaneTitle:   "⠋ Working",
		PaneContent: "",
	}
	assert.Equal(t, tmux.StatusRunning, tmux.ResolveStatus(input))
}

func TestResolveStatus_TitleDone_FallsToContent(t *testing.T) {
	input := tmux.StatusInput{
		HookStatus:  nil,
		PaneTitle:   "✓ Done",
		PaneContent: ">",
	}
	assert.Equal(t, tmux.StatusWaiting, tmux.ResolveStatus(input))
}

func TestResolveStatus_ContentFallback(t *testing.T) {
	input := tmux.StatusInput{
		HookStatus:  nil,
		PaneTitle:   "bash",
		PaneContent: "Processing\n  ctrl+c to interrupt",
	}
	assert.Equal(t, tmux.StatusRunning, tmux.ResolveStatus(input))
}

func TestResolveStatus_AllEmpty(t *testing.T) {
	input := tmux.StatusInput{
		HookStatus:  nil,
		PaneTitle:   "",
		PaneContent: "user@host:~$",
	}
	assert.Equal(t, tmux.StatusIdle, tmux.ResolveStatus(input))
}

func TestResolveStatus_ExpiredHook_Ignored(t *testing.T) {
	expired := &tmux.HookStatus{Status: tmux.StatusRunning, Timestamp: timeNowUnix() - 7200, RawStatus: "running"}
	input := tmux.StatusInput{
		HookStatus:  expired,
		PaneTitle:   "",
		PaneContent: ">",
	}
	assert.Equal(t, tmux.StatusWaiting, tmux.ResolveStatus(input))
}
