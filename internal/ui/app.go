package ui

import (
	"fmt"
	"strings"
	"time"

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

		// 狀態偵測
		for i := range sessions {
			sessions[i].Status = detectSessionStatus(deps, sessions[i].Name)
		}

		// 從 store 載入 groups 和 session metas
		var groups []store.Group
		if deps.Store != nil {
			groups, _ = deps.Store.ListGroups()
			metas, _ := deps.Store.ListAllSessionMetas()

			groupMap := make(map[int64]string)
			for _, g := range groups {
				groupMap[g.ID] = g.Name
			}

			metaMap := make(map[string]int64)
			for _, meta := range metas {
				metaMap[meta.SessionName] = meta.GroupID
			}

			for i := range sessions {
				if gid, ok := metaMap[sessions[i].Name]; ok {
					if name, ok := groupMap[gid]; ok {
						sessions[i].GroupName = name
					}
				}
			}
		}

		return SessionsMsg{Sessions: sessions, Groups: groups}
	}
}

// TickMsg 表示定時更新觸發。
type TickMsg struct{}

// tickCmd 排程下一次 tick。
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// detectSessionStatus 整合三層狀態偵測，回傳 session 的實際狀態。
func detectSessionStatus(deps Deps, sessionName string) tmux.SessionStatus {
	var input tmux.StatusInput

	// 第一層：Hook 狀態檔案
	if deps.StatusDir != "" {
		if hs, err := tmux.ReadHookStatus(deps.StatusDir, sessionName); err == nil {
			input.HookStatus = &hs
		}
	}

	// 第二層 + 第三層：只在 hook 無效時執行
	if input.HookStatus == nil || !input.HookStatus.IsValid() {
		if deps.TmuxMgr != nil {
			if content, err := deps.TmuxMgr.CapturePane(sessionName, 50); err == nil {
				input.PaneContent = content
			}
		}
	}

	return tmux.ResolveStatus(input)
}

// Init 實作 tea.Model 介面。
func (m Model) Init() tea.Cmd {
	interval := time.Duration(m.deps.Cfg.PollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return tea.Batch(
		tea.SetWindowTitle("tmux session menu"),
		loadSessionsCmd(m.deps),
		tickCmd(interval),
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
	case TickMsg:
		interval := time.Duration(m.deps.Cfg.PollIntervalSec) * time.Second
		if interval <= 0 {
			interval = 2 * time.Second
		}
		return m, tea.Batch(loadSessionsCmd(m.deps), tickCmd(interval))
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
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				item := m.items[m.cursor]
				if item.Type == ItemSession {
					m.selected = item.Session.Name
					m.quitting = true
					return m, tea.Quit
				}
			}
			return m, nil
		case "tab":
			if m.cursor >= 0 && m.cursor < len(m.items) {
				item := m.items[m.cursor]
				if item.Type == ItemGroup && m.deps.Store != nil {
					m.deps.Store.ToggleGroupCollapsed(item.Group.ID)
					return m, loadSessionsCmd(m.deps)
				}
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
