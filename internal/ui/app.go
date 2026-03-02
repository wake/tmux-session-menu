package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// Deps 封裝 Model 的外部依賴。
type Deps struct {
	TmuxMgr   *tmux.Manager
	Store     *store.Store
	Cfg       config.Config
	StatusDir string
}

// Model 是 Bubble Tea 的主要模型。
type Model struct {
	width    int
	height   int
	cursor   int
	items    []ListItem
	quitting bool

	deps     Deps
	selected string // Enter 選取的 session name
	err      error
}

// NewModel 建立初始 Model。
func NewModel(deps Deps) Model {
	return Model{deps: deps}
}

// SessionsMsg 是 loadSessions 完成後的回傳訊息。
type SessionsMsg struct {
	Sessions []tmux.Session
	Groups   []store.Group
	Err      error
}

// Items 回傳目前的列表項目。
func (m Model) Items() []ListItem {
	return m.items
}

// Selected 回傳使用者按 Enter 選取的 session name（空字串表示未選取）。
func (m Model) Selected() string {
	return m.selected
}

// loadSessionsCmd 建立一個 tea.Cmd，透過 TmuxMgr 載入 session 列表。
func loadSessionsCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		if deps.TmuxMgr == nil {
			return SessionsMsg{}
		}
		sessions, err := deps.TmuxMgr.ListSessions()
		if err != nil {
			return SessionsMsg{Err: err}
		}
		return SessionsMsg{Sessions: sessions}
	}
}

// Init 實作 tea.Model 介面。
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("tmux session menu"),
		loadSessionsCmd(m.deps),
	)
}

// Update 處理訊息並更新模型狀態。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case SessionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.items = FlattenItems(msg.Groups, msg.Sessions)
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		}
	}
	return m, nil
}

// View 渲染 TUI 畫面。
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("tmux session menu"))
	b.WriteString(dimStyle.Render("  (↑↓/jk 選擇, Enter 確認, q 離開)"))
	b.WriteString("\n")

	// Items list
	if len(m.items) > 0 {
		b.WriteString("\n")
		for i, item := range m.items {
			cursor := "  "
			if i == m.cursor {
				cursor = selectedStyle.Render("► ")
			}

			switch item.Type {
			case ItemGroup:
				collapse := "▼"
				if item.Group.Collapsed {
					collapse = "▶"
				}
				b.WriteString(fmt.Sprintf("%s%s %s\n",
					cursor,
					selectedStyle.Render(collapse),
					selectedStyle.Render(item.Group.Name)))

			case ItemSession:
				icon := item.Session.StatusIcon()
				styledIcon := statusStyleFor(item.Session.Status).Render(icon)

				relTime := ""
				if !item.Session.Activity.IsZero() {
					relTime = "  " + dimStyle.Render(item.Session.RelativeTime())
				}

				aiModel := ""
				if item.Session.AIModel != "" {
					aiModel = "  " + dimStyle.Render(item.Session.AIModel)
				}

				name := item.Session.Name
				if i == m.cursor {
					name = selectedStyle.Render(name)
				}

				b.WriteString(fmt.Sprintf("%s   %s  %s%s%s\n",
					cursor, name, styledIcon, relTime, aiModel))
			}
		}
	}

	// Help bar
	b.WriteString(fmt.Sprintf("\n  %s  %s  %s\n",
		dimStyle.Render("[n] 新建"),
		dimStyle.Render("[g] 新群組"),
		dimStyle.Render("[q] 離開")))

	// Preview section
	if len(m.items) > 0 && m.cursor >= 0 && m.cursor < len(m.items) {
		selected := m.items[m.cursor]
		if selected.Type == ItemSession && selected.Session.AISummary != "" {
			b.WriteString("\n")
			b.WriteString(previewBorderStyle.Render(
				fmt.Sprintf("Preview: %s", selected.Session.AISummary)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// statusStyleFor 回傳對應狀態的 lipgloss 樣式。
func statusStyleFor(status tmux.SessionStatus) lipgloss.Style {
	switch status {
	case tmux.StatusRunning:
		return statusRunningStyle
	case tmux.StatusWaiting:
		return statusWaitingStyle
	case tmux.StatusError:
		return statusErrorStyle
	default:
		return statusIdleStyle
	}
}

// SetItems 設定列表項目（主要用於測試）。
func (m *Model) SetItems(items []ListItem) {
	m.items = items
	if m.cursor >= len(items) && len(items) > 0 {
		m.cursor = len(items) - 1
	}
}

// Cursor 回傳目前游標位置。
func (m Model) Cursor() int {
	return m.cursor
}
