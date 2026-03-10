package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// uploadResult 代表一次上傳事件的顯示資料。
type uploadResult struct {
	files []*tsmv1.UploadedFile
	err   string
}

// uploadModal 是上傳模式的 TUI 元件。
type uploadModal struct {
	sessionName string
	hostLabel   string
	targetPath  string
	results     []uploadResult
}

func newUploadModal(sessionName, hostLabel, targetPath string) uploadModal {
	return uploadModal{
		sessionName: sessionName,
		hostLabel:   hostLabel,
		targetPath:  targetPath,
	}
}

func (m *uploadModal) addEvent(event *tsmv1.UploadEvent) {
	m.results = append(m.results, uploadResult{
		files: event.Files,
		err:   event.Error,
	})
}

func (m uploadModal) view() string {
	var b strings.Builder

	title := headerStyle.Render("上傳模式")
	b.WriteString(title)
	b.WriteString("\n\n")

	if m.hostLabel != "" {
		b.WriteString(fmt.Sprintf("  目標: %s %s\n", m.hostLabel, m.targetPath))
	} else {
		b.WriteString(fmt.Sprintf("  目標: %s\n", m.targetPath))
	}
	b.WriteString(fmt.Sprintf("  (session: %s)\n", m.sessionName))
	b.WriteString("\n")

	if len(m.results) == 0 {
		b.WriteString(dimStyle.Render("  拖曳檔案到 iTerm2 視窗即可上傳"))
		b.WriteString("\n")
	} else {
		for _, r := range m.results {
			if r.err != "" {
				b.WriteString(errorStyle.Render(fmt.Sprintf("  ✗ %s", r.err)))
				b.WriteString("\n")
				continue
			}
			for _, f := range r.files {
				sizeStr := formatUploadSize(f.SizeBytes)
				name := f.RemotePath
				if len(name) > 40 {
					name = "..." + name[len(name)-37:]
				}
				b.WriteString(successStyle.Render(fmt.Sprintf("  ✓ %-40s %s", name, sizeStr)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  [Esc] 關閉"))

	return b.String()
}

// updateUpload 處理上傳模式的按鍵。
func (m Model) updateUpload(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		sessionName := m.uploadModal.sessionName
		m.mode = ModeNormal
		return m, m.setUploadModeCmd(false, sessionName)
	}
	return m, nil
}

// setUploadModeCmd 透過 gRPC 設定上傳模式。
func (m Model) setUploadModeCmd(enabled bool, sessionName string) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = m.deps.Client.SetUploadMode(ctx, enabled, sessionName)
		}
		return nil
	}
}

func formatUploadSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
