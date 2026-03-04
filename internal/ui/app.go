package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/ai"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/version"
)

// Deps 封裝 Model 的外部依賴。
// 當 Client 不為 nil 時使用 gRPC daemon 模式；
// 否則使用舊的 TmuxMgr+Store 直接模式（主要用於測試）。
type Deps struct {
	Client    *client.Client // gRPC client（daemon 模式）
	TmuxMgr   *tmux.Manager  // 舊模式：直接 tmux 操作
	Store     *store.Store   // 舊模式：直接 SQLite 操作
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
	selected  string // Enter 選取的 session name
	exitTmux_ bool   // e 鍵：退出 tmux
	err       error

	// Mode 相關
	mode        Mode
	inputTarget InputTarget
	inputPrompt string
	textInput   textinput.Model

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
	ti := textinput.New()
	ti.PromptStyle = selectedStyle
	ti.TextStyle = lipgloss.NewStyle()
	ti.Cursor.Style = selectedStyle
	return Model{deps: deps, textInput: ti}
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

// ExitTmux 回傳使用者是否按了 e 要求退出 tmux。
func (m Model) ExitTmux() bool {
	return m.exitTmux_
}

// Mode 回傳目前的互動模式。
func (m Model) Mode() Mode {
	return m.mode
}

// InputTarget 回傳目前的輸入目標（主要用於測試）。
func (m Model) InputTarget() InputTarget {
	return m.inputTarget
}

// InputValue 回傳目前的輸入值（主要用於測試）。
func (m Model) InputValue() string {
	return m.textInput.Value()
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
			sortOrderMap := make(map[string]int)
			for _, meta := range metas {
				metaMap[meta.SessionName] = meta.GroupID
				sortOrderMap[meta.SessionName] = meta.SortOrder
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
				if so, ok := sortOrderMap[sessions[i].Name]; ok {
					sessions[i].SortOrder = so
				}
				if cn, ok := customNameMap[sessions[i].Name]; ok {
					sessions[i].CustomName = cn
				}
			}
		}

		return SessionsMsg{Sessions: sessions, Groups: groups}
	}
}

// SnapshotMsg 是從 gRPC Watch stream 接收的快照訊息。
type SnapshotMsg struct {
	Snapshot *tsmv1.StateSnapshot
	Err      error
}

// reconnectMsg 觸發 Watch stream 重連。
type reconnectMsg struct{}

// errMsg 封裝非同步操作回傳的錯誤。
type errMsg struct{ err error }

// TickMsg 表示定時更新觸發（舊模式使用）。
type TickMsg struct{}

// tickCmd 排程下一次 tick。
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// watchCmd 啟動 Watch stream 並持續接收快照。
func watchCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		if err := c.Watch(context.Background()); err != nil {
			return SnapshotMsg{Err: err}
		}
		return recvSnapshotCmd(c)()
	}
}

// recvSnapshotCmd 從 Watch stream 接收下一個快照。
func recvSnapshotCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		snap, err := c.RecvSnapshot()
		if err != nil {
			return SnapshotMsg{Err: err}
		}
		return SnapshotMsg{Snapshot: snap}
	}
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
	if m.deps.Client != nil {
		// gRPC daemon 模式：啟動 Watch stream
		return tea.Batch(
			tea.SetWindowTitle("tmux session menu"),
			watchCmd(m.deps.Client),
		)
	}
	// 舊模式：直接輪詢
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
	case SnapshotMsg:
		if msg.Err != nil {
			m.err = msg.Err
			// Watch stream 斷線後延遲 2 秒重連
			if m.deps.Client != nil {
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return reconnectMsg{}
				})
			}
			return m, nil
		}
		sessions := ConvertProtoSessions(msg.Snapshot.Sessions)
		groups := ConvertProtoGroups(msg.Snapshot.Groups)
		m.items = FlattenItems(groups, sessions)
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		// 繼續接收下一個快照
		if m.deps.Client != nil {
			return m, recvSnapshotCmd(m.deps.Client)
		}
		return m, nil
	case SessionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		cursorName, cursorIsGroup := m.cursorItemKey()
		m.items = FlattenItems(msg.Groups, msg.Sessions)
		m.restoreCursor(cursorName, cursorIsGroup)
		return m, nil
	case reconnectMsg:
		if m.deps.Client != nil {
			m.err = nil
			return m, watchCmd(m.deps.Client)
		}
		return m, nil
	case errMsg:
		m.err = msg.err
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
	case "e":
		m.quitting = true
		m.exitTmux_ = true
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
			switch item.Type {
			case ItemSession:
				m.selected = item.Session.Name
				m.quitting = true
				return m, tea.Quit
			case ItemGroup:
				return m.toggleCollapse(item)
			}
		}
		return m, nil
	case "tab":
		if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
			return m.toggleCollapse(m.items[m.cursor])
		}
		return m, nil
	case "left", "right":
		// 群組上切換展開/收合
		if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
			return m.toggleCollapse(m.items[m.cursor])
		}
		return m, nil
	case "n":
		m.mode = ModeInput
		m.inputTarget = InputNewSession
		m.inputPrompt = "Session 名稱"
		m.textInput.SetValue("")
		m.textInput.Focus()
		return m, nil
	case "r":
		// 重命名：session 設定自訂名稱，群組重命名
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			switch item.Type {
			case ItemSession:
				m.mode = ModeInput
				m.inputTarget = InputRenameSession
				m.inputPrompt = "自訂名稱"
				m.textInput.SetValue(item.Session.CustomName)
				m.textInput.Focus()
			case ItemGroup:
				m.mode = ModeInput
				m.inputTarget = InputRenameGroup
				m.inputPrompt = "群組名稱"
				m.textInput.SetValue(item.Group.Name)
				m.textInput.Focus()
			}
		}
		return m, nil
	case "g":
		// 新建群組：需要 Store 或 Client
		if m.deps.Client != nil || m.deps.Store != nil {
			m.mode = ModeInput
			m.inputTarget = InputNewGroup
			m.inputPrompt = "群組名稱"
			m.textInput.SetValue("")
			m.textInput.Focus()
		}
		return m, nil
	case "m":
		// 移動 session 到群組：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) && (m.deps.Client != nil || m.deps.Store != nil) {
			item := m.items[m.cursor]
			if item.Type == ItemSession {
				groups := m.collectGroups()
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
	case "c":
		// 清除 session 自訂名稱：僅對有 CustomName 的 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Type == ItemSession && item.Session.CustomName != "" {
				name := item.Session.Name
				if m.deps.Client != nil {
					if err := m.deps.Client.RenameSession(context.Background(), name, ""); err != nil {
						m.err = err
					}
					return m, nil // 變更透過 Watch stream 自動推送
				}
				if m.deps.Store != nil {
					if err := m.deps.Store.SetCustomName(name, ""); err != nil {
						m.err = err
						return m, nil
					}
					return m, loadSessionsCmd(m.deps)
				}
			}
		}
		return m, nil
	case "/":
		// 進入搜尋模式
		m.mode = ModeSearch
		m.searchQuery = ""
		m.cursor = 0
		return m, nil
	case "J":
		return m.moveItem(1) // 下移
	case "K":
		return m.moveItem(-1) // 上移
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
					if deps.Client != nil {
						if err := deps.Client.KillSession(context.Background(), name); err != nil {
							return func() tea.Msg { return errMsg{err} }
						}
						return nil // 變更透過 Watch stream 自動推送
					}
					if deps.TmuxMgr != nil {
						if err := deps.TmuxMgr.KillSession(name); err != nil {
							return func() tea.Msg { return errMsg{err} }
						}
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
		m.textInput.SetValue("")
		m.textInput.Blur()
		m.inputPrompt = ""
		return m, nil
	case tea.KeyEnter:
		value := strings.TrimSpace(m.textInput.Value())
		m.textInput.SetValue("")
		m.textInput.Blur()
		m.inputPrompt = ""
		if value == "" {
			m.mode = ModeNormal
			return m, nil
		}
		switch m.inputTarget {
		case InputNewSession:
			m.mode = ModeNormal
			if m.deps.Client != nil {
				if err := m.deps.Client.CreateSession(context.Background(), value, ""); err != nil {
					m.err = err
					return m, nil
				}
				return m, nil // 變更透過 Watch stream 自動推送
			}
			if m.deps.TmuxMgr != nil {
				if err := m.deps.TmuxMgr.NewSession(value, ""); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputRenameSession:
			m.mode = ModeNormal
			if m.deps.Client != nil {
				if m.cursor >= 0 && m.cursor < len(m.items) {
					name := m.items[m.cursor].Session.Name
					if err := m.deps.Client.RenameSession(context.Background(), name, value); err != nil {
						m.err = err
						return m, nil
					}
				}
				return m, nil // 變更透過 Watch stream 自動推送
			}
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
			if m.deps.Client != nil {
				if err := m.deps.Client.CreateGroup(context.Background(), value, 0); err != nil {
					m.err = err
					return m, nil
				}
				return m, nil // 變更透過 Watch stream 自動推送
			}
			if m.deps.Store != nil {
				if err := m.deps.Store.CreateGroup(value, 0); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputRenameGroup:
			m.mode = ModeNormal
			if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
				groupID := m.items[m.cursor].Group.ID
				if m.deps.Client != nil {
					if err := m.deps.Client.RenameGroup(context.Background(), groupID, value); err != nil {
						m.err = err
						return m, nil
					}
					return m, nil // 變更透過 Watch stream 自動推送
				}
				if m.deps.Store != nil {
					if err := m.deps.Store.RenameGroup(groupID, value); err != nil {
						m.err = err
						return m, nil
					}
				}
			}
			return m, loadSessionsCmd(m.deps)
		default:
			m.mode = ModeNormal
		}
		return m, nil
	default:
		// 委託給 textinput 處理所有其他按鍵（含空格、方向鍵、Backspace 等）
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
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
			if m.deps.Client != nil {
				if err := m.deps.Client.MoveSession(context.Background(), m.pickerTarget, group.ID, 0); err != nil {
					m.err = err
				}
			} else if m.deps.Store != nil {
				if err := m.deps.Store.SetSessionGroup(m.pickerTarget, group.ID, 0); err != nil {
					m.err = err
				}
			}
		}
		m.mode = ModeNormal
		m.pickerGroups = nil
		m.pickerTarget = ""
		if m.deps.Client != nil {
			return m, nil // 變更透過 Watch stream 自動推送
		}
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
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
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

// toggleCollapse 切換群組的展開/收合狀態。
func (m Model) toggleCollapse(item ListItem) (tea.Model, tea.Cmd) {
	if m.deps.Client != nil {
		if err := m.deps.Client.ToggleCollapse(context.Background(), item.Group.ID); err != nil {
			m.err = err
		}
		return m, nil // 變更透過 Watch stream 自動推送
	}
	if m.deps.Store != nil {
		if err := m.deps.Store.ToggleGroupCollapsed(item.Group.ID); err != nil {
			m.err = err
			return m, nil
		}
		return m, loadSessionsCmd(m.deps)
	}
	return m, nil
}

// collectGroups 從目前的 items 中收集所有群組。
func (m Model) collectGroups() []store.Group {
	if m.deps.Store != nil {
		groups, _ := m.deps.Store.ListGroups()
		return groups
	}
	// 從已有的 items 中提取
	var groups []store.Group
	for _, item := range m.items {
		if item.Type == ItemGroup {
			groups = append(groups, item.Group)
		}
	}
	return groups
}

// moveItem 根據方向移動目前選取的項目（群組或 session）。
func (m Model) moveItem(direction int) (tea.Model, tea.Cmd) {
	if m.deps.Client == nil && m.deps.Store == nil {
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return m, nil
	}
	item := m.items[m.cursor]
	switch item.Type {
	case ItemGroup:
		return m.moveGroup(item.Group, direction)
	case ItemSession:
		return m.moveSession(item.Session, direction)
	}
	return m, nil
}

// normalizeGroupOrders 當群組的 sort_order 有重複時，重新指派為連續值。
func (m Model) normalizeGroupOrders(groups []store.Group) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, g := range groups {
		if seen[g.SortOrder] {
			needsNormalize = true
			break
		}
		seen[g.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	for i, g := range groups {
		if m.deps.Client != nil {
			if err := m.deps.Client.ReorderGroup(context.Background(), g.ID, i); err != nil {
				return err
			}
		} else if m.deps.Store != nil {
			if err := m.deps.Store.SetGroupOrder(g.ID, i); err != nil {
				return err
			}
		}
		groups[i].SortOrder = i
	}
	return nil
}

// moveGroup 將群組在排序中上移或下移，交換 sort_order。
func (m Model) moveGroup(group store.Group, direction int) (tea.Model, tea.Cmd) {
	groups := m.collectGroups()
	idx := -1
	for i, g := range groups {
		if g.ID == group.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m, nil
	}
	newIdx := idx + direction
	if newIdx < 0 || newIdx >= len(groups) {
		return m, nil
	}

	// sort_order 重複時先正規化
	if err := m.normalizeGroupOrders(groups); err != nil {
		m.err = err
		return m, nil
	}

	if m.deps.Client != nil {
		// gRPC 模式：交換 sort_order
		if err := m.deps.Client.ReorderGroup(context.Background(), groups[idx].ID, groups[newIdx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		if err := m.deps.Client.ReorderGroup(context.Background(), groups[newIdx].ID, groups[idx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		m.cursor += direction
		return m, nil // 變更透過 Watch stream 自動推送
	}

	// 舊模式
	if err := m.deps.Store.SetGroupOrder(groups[idx].ID, groups[newIdx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	if err := m.deps.Store.SetGroupOrder(groups[newIdx].ID, groups[idx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	m.cursor += direction
	return m, loadSessionsCmd(m.deps)
}

// normalizeSessionOrders 當同群組 session 的 sort_order 有重複時，重新指派為連續值。
// gRPC 模式使用 siblings（來自 items），舊模式使用 metas（來自 store）。
func (m Model) normalizeSessionOrders(siblings []tmux.Session, groupID int64) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, s := range siblings {
		if seen[s.SortOrder] {
			needsNormalize = true
			break
		}
		seen[s.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	for i, s := range siblings {
		if m.deps.Client != nil {
			if err := m.deps.Client.ReorderSession(context.Background(), s.Name, groupID, i); err != nil {
				return err
			}
		} else if m.deps.Store != nil {
			if err := m.deps.Store.SetSessionGroup(s.Name, groupID, i); err != nil {
				return err
			}
		}
		siblings[i].SortOrder = i
	}
	return nil
}

// normalizeMetaOrders 當 session metas 的 sort_order 有重複時，重新指派為連續值（舊模式）。
func (m Model) normalizeMetaOrders(metas []store.SessionMeta, groupID int64) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, meta := range metas {
		if seen[meta.SortOrder] {
			needsNormalize = true
			break
		}
		seen[meta.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	for i, meta := range metas {
		if err := m.deps.Store.SetSessionGroup(meta.SessionName, groupID, i); err != nil {
			return err
		}
		metas[i].SortOrder = i
	}
	return nil
}

// moveSession 將 session 在群組內（或未分組）排序中上移或下移，交換 sort_order。
func (m Model) moveSession(session tmux.Session, direction int) (tea.Model, tea.Cmd) {
	var groupID int64
	if session.GroupName == "" {
		groupID = 0
	} else {
		groups := m.collectGroups()
		for _, g := range groups {
			if g.Name == session.GroupName {
				groupID = g.ID
				break
			}
		}
		if groupID == 0 {
			return m, nil
		}
	}

	if m.deps.Client != nil {
		// gRPC 模式：需要從 items 中找同群組的 session 來計算排序
		var siblings []tmux.Session
		for _, item := range m.items {
			if item.Type == ItemSession && item.Session.GroupName == session.GroupName {
				siblings = append(siblings, item.Session)
			}
		}
		idx := -1
		for i, s := range siblings {
			if s.Name == session.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return m, nil
		}
		newIdx := idx + direction
		if newIdx < 0 || newIdx >= len(siblings) {
			return m, nil
		}
		// sort_order 重複時先正規化
		if err := m.normalizeSessionOrders(siblings, groupID); err != nil {
			m.err = err
			return m, nil
		}
		if err := m.deps.Client.ReorderSession(context.Background(), siblings[idx].Name, groupID, siblings[newIdx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		if err := m.deps.Client.ReorderSession(context.Background(), siblings[newIdx].Name, groupID, siblings[idx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		newCursor := m.cursor + direction
		m.items[m.cursor], m.items[newCursor] = m.items[newCursor], m.items[m.cursor]
		m.cursor = newCursor
		return m, nil // 變更透過 Watch stream 自動推送
	}

	// 舊模式
	metas, err := m.deps.Store.ListSessionMetas(groupID)
	if err != nil {
		m.err = err
		return m, nil
	}
	idx := -1
	for i, meta := range metas {
		if meta.SessionName == session.Name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m, nil
	}
	newIdx := idx + direction
	if newIdx < 0 || newIdx >= len(metas) {
		return m, nil
	}
	// sort_order 重複時先正規化
	if err := m.normalizeMetaOrders(metas, groupID); err != nil {
		m.err = err
		return m, nil
	}
	// 交換 sort_order
	if err := m.deps.Store.SetSessionGroup(metas[idx].SessionName, groupID, metas[newIdx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	if err := m.deps.Store.SetSessionGroup(metas[newIdx].SessionName, groupID, metas[idx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	m.cursor += direction
	return m, loadSessionsCmd(m.deps)
}

// cursorItemKey 回傳 cursor 目前指向的 item 識別資訊。
func (m Model) cursorItemKey() (name string, isGroup bool) {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		item := m.items[m.cursor]
		if item.Type == ItemSession {
			return item.Session.Name, false
		}
		if item.Type == ItemGroup {
			return item.Group.Name, true
		}
	}
	return "", false
}

// restoreCursor 嘗試將 cursor 還原到指定 item 位置。
func (m *Model) restoreCursor(name string, isGroup bool) {
	if name != "" {
		for i, item := range m.items {
			if !isGroup && item.Type == ItemSession && item.Session.Name == name {
				m.cursor = i
				return
			}
			if isGroup && item.Type == ItemGroup && item.Group.Name == name {
				m.cursor = i
				return
			}
		}
	}
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
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
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString(dimStyle.Render("  (↑↓/jk 選擇, Enter 確認, q 離開)"))
	b.WriteString("\n")

	// Items list
	displayItems := m.visibleItems()
	if len(displayItems) > 0 {
		b.WriteString("\n")

		// 計算主行號總數（群組 + 未分組 session）以決定對齊位數
		mainLineCount := 0
		for _, item := range displayItems {
			isChild := item.Type == ItemSession && item.Session.GroupName != ""
			if !isChild {
				mainLineCount++
			}
		}
		maxDigits := len(fmt.Sprintf("%d", mainLineCount))

		lineNum := 0
		for i, item := range displayItems {
			isGroupChild := item.Type == ItemSession && item.Session.GroupName != ""

			if !isGroupChild {
				lineNum++
			}

			// 前綴：cursor 箭頭 / 行號+中點 / 子項目空格對齊
			var prefix string
			if i == m.cursor {
				prefix = selectedStyle.Render("►") + " "
			} else if isGroupChild {
				prefix = strings.Repeat(" ", maxDigits+1) // 對齊行號+中點寬度
			} else {
				numStr := fmt.Sprintf("%d", lineNum)
				prefix = dimStyle.Render(numStr+"·") + strings.Repeat(" ", maxDigits-len(numStr))
			}

			switch item.Type {
			case ItemGroup:
				collapse := "▾"
				if item.Group.Collapsed {
					collapse = "▸"
				}
				if i == m.cursor {
					b.WriteString(fmt.Sprintf("%s%s %s\n",
						prefix,
						selectedStyle.Render(collapse),
						selectedStyle.Render(item.Group.Name)))
				} else {
					b.WriteString(fmt.Sprintf("%s%s %s\n",
						prefix,
						dimStyle.Render(collapse),
						groupNameStyle.Render(item.Group.Name)))
				}

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
				} else {
					name = sessionNameStyle.Render(name)
				}

				if isGroupChild {
					// 樹狀符號：判斷是否為群組最後一個子項目
					isLast := true
					if i+1 < len(displayItems) {
						next := displayItems[i+1]
						if next.Type == ItemSession && next.Session.GroupName == item.Session.GroupName {
							isLast = false
						}
					}
					tree := dimStyle.Render("├─")
					if isLast {
						tree = dimStyle.Render("└─")
					}
					b.WriteString(fmt.Sprintf("%s%s %s %s%s%s\n",
						prefix, tree, styledIcon, name, relTime, aiModel))
				} else {
					b.WriteString(fmt.Sprintf("%s%s %s%s%s\n",
						prefix, styledIcon, name, relTime, aiModel))
				}
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
		b.WriteString(fmt.Sprintf("\n  %s: %s\n",
			selectedStyle.Render(m.inputPrompt),
			m.textInput.View()))
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
		b.WriteString(fmt.Sprintf("\n  %s%s  %s%s  %s%s  %s%s  %s%s  %s%s  %s%s  %s%s  %s%s\n",
			keyStyle.Render("[n]"), dimStyle.Render(" 新建"),
			keyStyle.Render("[d]"), dimStyle.Render(" 刪除"),
			keyStyle.Render("[r]"), dimStyle.Render(" 更名"),
			keyStyle.Render("[c]"), dimStyle.Render(" 清除名稱"),
			keyStyle.Render("[g]"), dimStyle.Render(" 群組"),
			keyStyle.Render("[m]"), dimStyle.Render(" 移動"),
			keyStyle.Render("[/]"), dimStyle.Render(" 搜尋"),
			keyStyle.Render("[e]"), dimStyle.Render(" 退出 tmux"),
			keyStyle.Render("[q]"), dimStyle.Render(" 離開")))
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

// SetCursor 設定游標位置（主要用於測試）。
func (m *Model) SetCursor(pos int) {
	m.cursor = pos
}
