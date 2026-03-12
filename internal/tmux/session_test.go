package tmux_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func TestSession_RelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		activity time.Time
		expected string
	}{
		{"just now", time.Now().Add(-30 * time.Second), "30s"},
		{"minutes", time.Now().Add(-5 * time.Minute), "5m"},
		{"hours", time.Now().Add(-3 * time.Hour), "3h"},
		{"days", time.Now().Add(-2 * 24 * time.Hour), "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tmux.Session{Activity: tt.activity}
			assert.Equal(t, tt.expected, s.RelativeTime())
		})
	}
}

func TestSession_StatusIcon(t *testing.T) {
	tests := []struct {
		name     string
		sess     tmux.Session
		expected string
	}{
		// 有 AiType：running/waiting/idle 用 ●，error 用 ✗
		{"ai running", tmux.Session{AiType: "claude", Status: tmux.StatusRunning}, "●"},
		{"ai waiting", tmux.Session{AiType: "claude", Status: tmux.StatusWaiting}, "●"},
		{"ai idle", tmux.Session{AiType: "claude", Status: tmux.StatusIdle}, "●"},
		{"ai error", tmux.Session{AiType: "claude", Status: tmux.StatusError}, "✗"},
		// 無 AiType：一律用 ○
		{"no ai running", tmux.Session{Status: tmux.StatusRunning}, "○"},
		{"no ai waiting", tmux.Session{Status: tmux.StatusWaiting}, "○"},
		{"no ai idle", tmux.Session{Status: tmux.StatusIdle}, "○"},
		{"no ai error", tmux.Session{Status: tmux.StatusError}, "○"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.sess.StatusIcon())
		})
	}
}

func TestParseListSessions(t *testing.T) {
	output := `my-project:$1:1:/home/user/project:1:1709312400
api-server:$2:0:/home/user/api:0:1709308800`

	sessions, err := tmux.ParseListSessions(output)
	assert.NoError(t, err)
	assert.Len(t, sessions, 2)

	assert.Equal(t, "my-project", sessions[0].Name)
	assert.Equal(t, "$1", sessions[0].ID)
	assert.Equal(t, "/home/user/project", sessions[0].Path)
	assert.True(t, sessions[0].Attached)

	assert.Equal(t, "api-server", sessions[1].Name)
	assert.Equal(t, "$2", sessions[1].ID)
	assert.Equal(t, "/home/user/api", sessions[1].Path)
	assert.False(t, sessions[1].Attached)
}

func TestParseListSessions_Empty(t *testing.T) {
	sessions, err := tmux.ParseListSessions("")
	assert.NoError(t, err)
	assert.Empty(t, sessions)
}

// mockExecutor 用於測試 Manager，模擬 tmux 指令的執行。
type mockExecutor struct {
	outputs map[string]string
	err     error
}

func (m *mockExecutor) Execute(args ...string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	key := strings.Join(args, " ")
	return m.outputs[key], nil
}

func TestManager_ListSessions(t *testing.T) {
	mock := &mockExecutor{
		outputs: map[string]string{
			"list-sessions -F #{session_name}:#{session_id}:#{session_windows}:#{session_path}:#{session_attached}:#{session_activity}": `work:$1:1:/home/user/work:1:1709312400`,
		},
	}

	mgr := tmux.NewManager(mock)
	sessions, err := mgr.ListSessions()

	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "work", sessions[0].Name)
}

func TestManager_KillSession(t *testing.T) {
	mock := &mockExecutor{outputs: map[string]string{
		"kill-session -t my-session": "",
	}}

	mgr := tmux.NewManager(mock)
	err := mgr.KillSession("my-session")
	assert.NoError(t, err)
}

func TestManager_RenameSession(t *testing.T) {
	mock := &mockExecutor{outputs: map[string]string{
		"rename-session -t old-name new-name": "",
	}}

	mgr := tmux.NewManager(mock)
	err := mgr.RenameSession("old-name", "new-name")
	assert.NoError(t, err)
}

func TestManager_NewSession(t *testing.T) {
	mock := &mockExecutor{outputs: map[string]string{
		"new-session -d -s test-session -c /tmp": "",
	}}

	mgr := tmux.NewManager(mock)
	err := mgr.NewSession("test-session", "/tmp")
	assert.NoError(t, err)
}

func TestManager_CapturePane(t *testing.T) {
	mock := &mockExecutor{outputs: map[string]string{
		"capture-pane -t my-session -p -S -150": "line 1\nline 2\nline 3",
	}}

	mgr := tmux.NewManager(mock)
	output, err := mgr.CapturePane("my-session", 150)

	assert.NoError(t, err)
	assert.Equal(t, "line 1\nline 2\nline 3", output)
}

func TestSession_DisplayName(t *testing.T) {
	// 無自訂名稱 → 回傳 Name
	s := tmux.Session{Name: "my-project"}
	assert.Equal(t, "my-project", s.DisplayName())

	// 有自訂名稱 → 回傳 CustomName
	s.CustomName = "我的專案"
	assert.Equal(t, "我的專案", s.DisplayName())
}

func TestManager_SendKeys(t *testing.T) {
	mock := &mockExecutor{outputs: map[string]string{
		"send-keys -t my-session claude Enter": "",
	}}

	mgr := tmux.NewManager(mock)
	err := mgr.SendKeys("my-session", "claude")
	assert.NoError(t, err)
}

func TestManager_ListPaneTitles(t *testing.T) {
	mock := &mockExecutor{
		outputs: map[string]string{
			"list-panes -a -F #{session_name}:#{pane_title}": "dev:⠋ Working\nops:✓ Done\n",
		},
	}
	mgr := tmux.NewManager(mock)
	titles, err := mgr.ListPaneTitles()
	assert.NoError(t, err)
	assert.Equal(t, "⠋ Working", titles["dev"])
	assert.Equal(t, "✓ Done", titles["ops"])
}
