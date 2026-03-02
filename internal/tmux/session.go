package tmux

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ListSessionsFormat 是傳給 tmux list-sessions -F 的格式字串。
const ListSessionsFormat = "#{session_name}:#{session_id}:#{session_windows}:#{session_path}:#{session_attached}:#{session_activity}"

// ParseListSessions 解析 tmux list-sessions 的輸出，回傳 Session 切片。
func ParseListSessions(output string) ([]Session, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	sessions := make([]Session, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 6)
		if len(parts) < 6 {
			return nil, fmt.Errorf("unexpected format: %q", line)
		}

		attached := parts[4] == "1"
		activityUnix, err := strconv.ParseInt(parts[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid activity timestamp %q: %w", parts[5], err)
		}

		sessions = append(sessions, Session{
			Name:     parts[0],
			ID:       parts[1],
			Path:     parts[3],
			Attached: attached,
			Activity: time.Unix(activityUnix, 0),
		})
	}

	return sessions, nil
}

// Manager 封裝 tmux 操作，透過 Executor 介面執行指令。
type Manager struct {
	exec Executor
}

// NewManager 建立新的 Manager。
func NewManager(exec Executor) *Manager {
	return &Manager{exec: exec}
}

// ListSessions 列出所有 tmux session。
func (m *Manager) ListSessions() ([]Session, error) {
	output, err := m.exec.Execute("list-sessions", "-F", ListSessionsFormat)
	if err != nil {
		return nil, err
	}
	return ParseListSessions(output)
}

// KillSession 刪除指定的 session。
func (m *Manager) KillSession(name string) error {
	_, err := m.exec.Execute("kill-session", "-t", name)
	return err
}

// RenameSession 重新命名 session。
func (m *Manager) RenameSession(oldName, newName string) error {
	_, err := m.exec.Execute("rename-session", "-t", oldName, newName)
	return err
}

// NewSession 建立新的 detached session。
func (m *Manager) NewSession(name, path string) error {
	_, err := m.exec.Execute("new-session", "-d", "-s", name, "-c", path)
	return err
}

// CapturePane 擷取指定 session 的 pane 內容。
func (m *Manager) CapturePane(name string, lines int) (string, error) {
	return m.exec.Execute("capture-pane", "-t", name, "-p", "-S", fmt.Sprintf("-%d", lines))
}

// ListPaneTitles 列出所有 pane 的 title，回傳 session name → pane title 的對應。
func (m *Manager) ListPaneTitles() (map[string]string, error) {
	output, err := m.exec.Execute("list-panes", "-a", "-F", PaneTitleFormat)
	if err != nil {
		return nil, err
	}
	return ParseListPaneTitles(output)
}
