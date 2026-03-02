package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/ai"
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

	// Mode 相關
	mode        Mode
	inputTarget InputTarget
	inputPrompt string
	inputValue  string

	confirmPrompt string
	confirmAction func() tea.Cmd

	// Picker mode（選擇群組）
	pickerGroups []store.Group
	pickerCursor int
	pickerTarget string // 被移動的 session name

	// Search mode（搜尋）
	searchQuery string
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

// Mode 回傳目前的互動模式。
func (m Model) Mode() Mode {
	return m.mode
}

// InputValue 回傳目前的輸入值（主要用於測試）。
func (m Model) InputValue() string {
	return m.inputValue
}

// Err 回傳目前的錯誤（主要用於測試）。
func (m Model) Err() error {
	return m.err
}

// SearchQuery 回傳目前的搜尋字串（主要用於測試）。
func (m Model) SearchQuery() string {
	return m.searchQuery
}

// visibleItems 回傳目前可見的項目（搜尋時過濾）。
func (m Model) visibleItems() []ListItem {
	if m.mode != ModeSearch || m.searchQuery == "" {
		return m.items
	}
	query := strings.ToLower(m.searchQuery)
	var filtered []ListItem
	for _, item := range m.items {
		if item.Type == ItemGroup {
			continue
		}
		if strings.Contains(strings.ToLower(item.Session.DisplayName()), query) ||
			strings.Contains(strings.ToLower(item.Session.Name), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
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

		// 擷取 pane 內容的行數，預設 150
		previewLines := deps.Cfg.PreviewLines
		if previewLines <= 0 {
			previewLines = 150
		}

		// 第二層：一次取得所有 pane title
		paneTitles := make(map[string]string)
		if deps.TmuxMgr != nil {
			if titles, err := deps.TmuxMgr.ListPaneTitles(); err == nil {
				paneTitles = titles
			}
		}

		// 狀態偵測與 AI 模型偵測
		for i := range sessions {
			var paneContent string
			if deps.TmuxMgr != nil {
				if content, err := deps.TmuxMgr.CapturePane(sessions[i].Name, previewLines); err == nil {
					paneContent = content
				}
			}
			paneTitle := paneTitles[sessions[i].Name]
			sessions[i].Status = detectSessionStatus(deps, sessions[i].Name, paneTitle, paneContent)
			sessions[i].AIModel = ai.DetectModel(paneContent)
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
			customNameMap := make(map[string]string)
			for _, meta := range metas {
				metaMap[meta.SessionName] = meta.GroupID
				if meta.CustomName != "" {
					customNameMap[meta.SessionName] = meta.CustomName
				}
			}

			for i := range sessions {
				if gid, ok := metaMap[sessions[i].Name]; ok {
					if name, ok := groupMap[gid]; ok {
						sessions[i].GroupName = name
					}
				}
				if cn, ok := customNameMap[sessions[i].Name]; ok {
					sessions[i].CustomName = cn
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
func detectSessionStatus(deps Deps, sessionName, paneTitle, paneContent string) tmux.SessionStatus {
	var input tmux.StatusInput

	// 第一層：Hook 狀態檔案
	if deps.StatusDir != "" {
		if hs, err := tmux.ReadHookStatus(deps.StatusDir, sessionName); err == nil {
			input.HookStatus = &hs
		}
	}

	// 第二層（pane title）+ 第三層（terminal content）：只在 hook 無效時執行
	if input.HookStatus == nil || !input.HookStatus.IsValid() {
		input.PaneTitle = paneTitle
		input.PaneContent = paneContent
	}

	return tmux.ResolveStatus(input)
}

// pollInterval 回傳設定的輪詢間隔，若未設定則預設 2 秒。
func (m Model) pollInterval() time.Duration {
	interval := time.Duration(m.deps.Cfg.PollIntervalSec) * time.Second
	if interval <= 0 {
		return 2 * time.Second
	}
	return interval
}

// Init 實作 tea.Model 介面。
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("tmux session menu"),
		loadSessionsCmd(m.deps),
		tickCmd(m.pollInterval()),
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
		return m, tea.Batch(loadSessionsCmd(m.deps), tickCmd(m.pollInterval()))
	case tea.KeyMsg:
		switch m.mode {
		case ModeInput:
			return m.updateInput(msg)
		case ModeConfirm:
			return m.updateConfirm(msg)
		case ModePicker:
			return m.updatePicker(msg)
		case ModeSearch:
			return m.updateSearch(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

// updateNormal 處理一般瀏覽模式的按鍵。
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
				if err := m.deps.Store.ToggleGroupCollapsed(item.Group.ID); err != nil {
					m.err = err
					return m, nil
				}
				return m, loadSessionsCmd(m.deps)
			}
		}
		return m, nil
	case "n":
		m.mode = ModeInput
		m.inputTarget = InputNewSession
		m.inputPrompt = "Session 名稱"
		m.inputValue = ""
		return m, nil
	case "r":
		// 重命名 session（自訂名稱）：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Type == ItemSession {
				m.mode = ModeInput
				m.inputTarget = InputRenameSession
				m.inputPrompt = "自訂名稱"
				m.inputValue = item.Session.CustomName
			}
		}
		return m, nil
	case "g":
		// 新建群組：需要 Store
		if m.deps.Store != nil {
			m.mode = ModeInput
			m.inputTarget = InputNewGroup
			m.inputPrompt = "群組名稱"
			m.inputValue = ""
		}
		return m, nil
	case "m":
		// 移動 session 到群組：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) && m.deps.Store != nil {
			item := m.items[m.cursor]
			if item.Type == ItemSession {
				groups, _ := m.deps.Store.ListGroups()
				if len(groups) == 0 {
					return m, nil
				}
				m.mode = ModePicker
				m.pickerGroups = groups
				m.pickerCursor = 0
				m.pickerTarget = item.Session.Name
			}
		}
		return m, nil
	case "/":
		// 進入搜尋模式
		m.mode = ModeSearch
		m.searchQuery = ""
		m.cursor = 0
		return m, nil
	case "d":
		// 刪除 session：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Type == ItemSession {
				name := item.Session.Name
				deps := m.deps
				m.mode = ModeConfirm
				m.confirmPrompt = fmt.Sprintf("確定要刪除 session %q？", name)
				m.confirmAction = func() tea.Cmd {
					if deps.TmuxMgr != nil {
						deps.TmuxMgr.KillSession(name)
					}
					return loadSessionsCmd(deps)
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateInput 處理文字輸入模式的按鍵。
func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// 取消輸入，回到一般模式
		m.mode = ModeNormal
		m.inputValue = ""
		m.inputPrompt = ""
		return m, nil
	case tea.KeyBackspace:
		// 刪除最後一個字元
		if len(m.inputValue) > 0 {
			runes := []rune(m.inputValue)
			m.inputValue = string(runes[:len(runes)-1])
		}
		return m, nil
	case tea.KeyEnter:
		value := strings.TrimSpace(m.inputValue)
		m.inputValue = ""
		m.inputPrompt = ""
		if value == "" {
			m.mode = ModeNormal
			return m, nil
		}
		switch m.inputTarget {
		case InputNewSession:
			m.mode = ModeNormal
			if m.deps.TmuxMgr != nil {
				if err := m.deps.TmuxMgr.NewSession(value, ""); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputRenameSession:
			m.mode = ModeNormal
			if m.deps.Store != nil && m.cursor >= 0 && m.cursor < len(m.items) {
				name := m.items[m.cursor].Session.Name
				if err := m.deps.Store.SetCustomName(name, value); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputNewGroup:
			m.mode = ModeNormal
			if m.deps.Store != nil {
				if err := m.deps.Store.CreateGroup(value, 0); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		default:
			m.mode = ModeNormal
		}
		return m, nil
	case tea.KeyRunes:
		// 附加輸入字元
		m.inputValue += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

// updateConfirm 處理確認對話框模式的按鍵。
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		// 執行確認動作
		var cmd tea.Cmd
		if m.confirmAction != nil {
			cmd = m.confirmAction()
			m.confirmAction = nil
		}
		m.mode = ModeNormal
		m.confirmPrompt = ""
		return m, cmd
	case "n", "N", "esc":
		// 取消確認
		m.mode = ModeNormal
		m.confirmPrompt = ""
		m.confirmAction = nil
		return m, nil
	}
	return m, nil
}

// updatePicker 處理群組選擇器模式的按鍵。
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.pickerCursor < len(m.pickerGroups)-1 {
			m.pickerCursor++
		}
	case "k", "up":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case "enter":
		if m.pickerCursor >= 0 && m.pickerCursor < len(m.pickerGroups) {
			group := m.pickerGroups[m.pickerCursor]
			if m.deps.Store != nil {
				if err := m.deps.Store.SetSessionGroup(m.pickerTarget, group.ID, 0); err != nil {
					m.err = err
				}
			}
		}
		m.mode = ModeNormal
		m.pickerGroups = nil
		m.pickerTarget = ""
		return m, loadSessionsCmd(m.deps)
	case "esc":
		m.mode = ModeNormal
		m.pickerGroups = nil
		m.pickerTarget = ""
	}
	return m, nil
}

// updateSearch 處理搜尋模式的按鍵。
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.mode = ModeNormal
		m.searchQuery = ""
		m.cursor = 0
	case tea.KeyEnter:
		visible := m.visibleItems()
		if m.cursor >= 0 && m.cursor < len(visible) {
			item := visible[m.cursor]
			if item.Type == ItemSession {
				m.selected = item.Session.Name
				m.quitting = true
				return m, tea.Quit
			}
		}
	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
		visible := m.visibleItems()
		if m.cursor >= len(visible) && len(visible) > 0 {
			m.cursor = len(visible) - 1
		}
	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.cursor = 0
	default:
		// j/k 導航
		switch msg.String() {
		case "j", "down":
			visible := m.visibleItems()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

// SetConfirm 設定確認模式（主要用於測試）。
func (m *Model) SetConfirm(prompt string, action func() tea.Cmd) {
	m.mode = ModeConfirm
	m.confirmPrompt = prompt
	m.confirmAction = action
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
	displayItems := m.visibleItems()
	if len(displayItems) > 0 {
		b.WriteString("\n")
		for i, item := range displayItems {
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

				name := item.Session.DisplayName()
				if i == m.cursor {
					name = selectedStyle.Render(name)
				}

				b.WriteString(fmt.Sprintf("%s   %s  %s%s%s\n",
					cursor, name, styledIcon, relTime, aiModel))
			}
		}
	}

	// Error display
	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  %s\n", statusErrorStyle.Render("Error: "+m.err.Error())))
	}

	// 模式相關的底部列
	switch m.mode {
	case ModeInput:
		b.WriteString(fmt.Sprintf("\n  %s: %s%s\n",
			selectedStyle.Render(m.inputPrompt),
			m.inputValue,
			selectedStyle.Render("\u2588"))) // 游標方塊字元
		b.WriteString(fmt.Sprintf("  %s\n",
			dimStyle.Render("[Esc] 取消")))
	case ModeConfirm:
		b.WriteString(fmt.Sprintf("\n  %s  %s\n",
			selectedStyle.Render(m.confirmPrompt),
			dimStyle.Render("[y] 確認  [n/Esc] 取消")))
	case ModePicker:
		b.WriteString(fmt.Sprintf("\n  %s\n", selectedStyle.Render("移動到群組：")))
		for i, g := range m.pickerGroups {
			cursor := "  "
			if i == m.pickerCursor {
				cursor = selectedStyle.Render("► ")
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", cursor, g.Name))
		}
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("[Enter] 確認  [Esc] 取消")))
	case ModeSearch:
		b.WriteString(fmt.Sprintf("\n  %s: %s█\n",
			selectedStyle.Render("/"),
			m.searchQuery))
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("[Enter] 選擇  [Esc] 取消")))
	default:
		b.WriteString(fmt.Sprintf("\n  %s  %s  %s\n",
			dimStyle.Render("[n] 新建"),
			dimStyle.Render("[g] 新群組"),
			dimStyle.Render("[q] 離開")))
	}

	// Preview section
	visible := m.visibleItems()
	if len(visible) > 0 && m.cursor >= 0 && m.cursor < len(visible) {
		selected := visible[m.cursor]
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

// PickerCursor 回傳 picker 模式的游標位置（主要用於測試）。
func (m Model) PickerCursor() int {
	return m.pickerCursor
}
