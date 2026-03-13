package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

// newSessionField 代表表單中的焦點區塊。
type newSessionField int

const (
	fieldHost   newSessionField = iota // 主機選擇（tab 樣式，多主機時才顯示）
	fieldName                          // 名稱（CustomName，可選）
	fieldID                            // ID（tmux session name，可選，留空則用名稱）
	fieldPath                          // 啟動路徑
	fieldRecent                        // 最近路徑選擇
	fieldAgent                         // Agent 選擇
)

// hostTabInfo 描述新建 session 時可選的主機。
type hostTabInfo struct {
	ID    string
	Name  string
	Color string
}

// agentGroup 是分群後的 agent 清單。
type agentGroup struct {
	title   string
	indices []int // 對應 agents 切片的 index
}

// newSessionForm 管理 new session 表單的狀態。
type newSessionForm struct {
	nameInput textinput.Model // 名稱（CustomName，顯示名稱）
	idInput   textinput.Model // ID（tmux session name）
	pathInput textinput.Model

	field       newSessionField // 目前焦點區塊
	recentPaths []string        // 最近路徑
	recentIdx   int             // 最近路徑的游標

	agents        []config.AgentEntry // 已啟用的 agent 清單
	agentGroups   []agentGroup        // 分群後的 agent
	agentCursor   int                 // agent 選項中的全域游標（含 Shell）
	selectedAgent int                 // 已選取的 agent index（-1 = Shell）

	hosts   []hostTabInfo // 可選主機（多主機模式）
	hostIdx int           // 選取的主機 index
}

func (f *newSessionForm) totalAgentOptions() int {
	return len(f.agents) + 1 // +1 for Shell
}

// hasHosts 回傳是否有多主機可選。
func (f *newSessionForm) hasHosts() bool {
	return len(f.hosts) > 1
}

// firstField 回傳表單的第一個可用焦點欄位。
func (f *newSessionForm) firstField() newSessionField {
	if f.hasHosts() {
		return fieldHost
	}
	return fieldName
}

// initForm 初始化表單。
func (f *newSessionForm) initForm(agents []config.AgentEntry, recentPaths []string, hosts []hostTabInfo) {
	f.nameInput = textinput.New()
	f.nameInput.Placeholder = "顯示名稱（可選）"

	f.idInput = textinput.New()
	f.idInput.Placeholder = "session-name（可選，留空則用名稱）"

	f.pathInput = textinput.New()
	f.pathInput.Placeholder = "~/Workspace/..."

	f.hosts = hosts
	f.hostIdx = 0

	f.recentPaths = recentPaths
	f.recentIdx = 0
	f.selectedAgent = -1 // Shell selected by default
	f.agentCursor = 0

	// 設定初始焦點
	f.field = f.firstField()
	if f.field == fieldName {
		f.nameInput.Focus()
	}

	// 過濾已啟用的 agent，按 SortOrder 排列
	var enabled []config.AgentEntry
	for _, a := range agents {
		if a.Enabled {
			enabled = append(enabled, a)
		}
	}
	f.agents = enabled

	// 依 Group 分群
	groupOrder := []string{}
	groupMap := make(map[string][]int)
	for i, a := range enabled {
		g := a.Group
		if _, exists := groupMap[g]; !exists {
			groupOrder = append(groupOrder, g)
		}
		groupMap[g] = append(groupMap[g], i)
	}
	f.agentGroups = nil
	for _, g := range groupOrder {
		f.agentGroups = append(f.agentGroups, agentGroup{
			title:   g,
			indices: groupMap[g],
		})
	}
}

// blurAllInputs 取消所有文字輸入的焦點。
func (f *newSessionForm) blurAllInputs() {
	f.nameInput.Blur()
	f.idInput.Blur()
	f.pathInput.Blur()
}

// selectedHostID 回傳目前選取的主機 ID（無主機時回傳空字串）。
func (f *newSessionForm) selectedHostID() string {
	if f.hostIdx >= 0 && f.hostIdx < len(f.hosts) {
		return f.hosts[f.hostIdx].ID
	}
	return ""
}

// updateNewSession 處理 ModeNewSession 的按鍵。
func (m Model) updateNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.newSession
	key := msg.String()

	switch key {
	case "esc":
		m.mode = ModeNormal
		return m, nil
	case "q":
		// 非文字輸入焦點時才關閉（避免攔截打字）
		if f.field != fieldName && f.field != fieldID && f.field != fieldPath {
			m.mode = ModeNormal
			return m, nil
		}

	case "tab":
		// Tab 鍵：在主機 tab 區域時切換到下一台主機
		if f.field == fieldHost && f.hasHosts() {
			f.hostIdx = (f.hostIdx + 1) % len(f.hosts)
			return m, nil
		}

	case "enter":
		displayName := strings.TrimSpace(f.nameInput.Value())
		sessionID := strings.TrimSpace(f.idInput.Value())

		// 決定 tmux session name：ID 優先，其次名稱
		sessionName := sessionID
		if sessionName == "" {
			sessionName = displayName
		}
		if sessionName == "" {
			return m, nil // 至少需要一個識別名稱
		}

		// tmux session name 不允許 '.' 和 ':'
		if strings.ContainsAny(sessionName, ".:") {
			m.err = fmt.Errorf("session name 不可包含 '.' 或 ':'")
			m.mode = ModeNormal
			return m, nil
		}

		path := f.pathInput.Value()
		command := ""
		if f.selectedAgent >= 0 && f.selectedAgent < len(f.agents) {
			command = f.agents[f.selectedAgent].Command
		}

		// 判斷是否需要設定自訂名稱（名稱不同於 session name 時才設定）
		customName := ""
		if displayName != "" && displayName != sessionName {
			customName = displayName
		}

		// 取得目標主機的 client
		hostID := f.selectedHostID()

		// Hub 模式：透過 ProxyMutation 路由到目標主機
		if m.deps.HubMode && m.deps.Client != nil {
			err := m.deps.Client.ProxyMutation(context.Background(), hostID,
				tsmv1.MutationType_MUTATION_CREATE_SESSION,
				sessionName, "", "", path, command)
			if err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			if customName != "" {
				_ = m.deps.Client.ProxyMutation(context.Background(), hostID,
					tsmv1.MutationType_MUTATION_RENAME_SESSION,
					sessionName, customName, "", "", "")
			}
		} else if client := m.clientForHost(hostID); client != nil {
			if err := client.CreateSession(context.Background(), sessionName, path, command); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			// 設定自訂名稱
			if customName != "" {
				_ = client.RenameSession(context.Background(), sessionName, customName, "")
			}
		} else if m.deps.TmuxMgr != nil {
			if err := m.deps.TmuxMgr.NewSession(sessionName, path); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			if command != "" {
				_ = m.deps.TmuxMgr.SendKeys(sessionName, command)
			}
			// 設定自訂名稱
			if customName != "" {
				if m.deps.Store != nil {
					_ = m.deps.Store.SetCustomName(sessionName, customName)
				}
				_ = m.deps.TmuxMgr.SetSessionOption(sessionName, "@tsm_name", customName)
			}
		}

		// 記錄路徑歷史
		if path != "" && m.deps.Store != nil {
			_ = m.deps.Store.AddPathHistory(path)
		}

		// 自動掛載：設定選取的 session 並退出 TUI
		m.selected = sessionName
		m.selectedHostID = hostID
		m.quitting = true
		return m, tea.Quit

	case "up":
		switch f.field {
		case fieldHost:
			// 已在最頂部
		case fieldName:
			if f.hasHosts() {
				f.blurAllInputs()
				f.field = fieldHost
			}
			// 無主機時已在最頂部
		case fieldID:
			f.field = fieldName
			f.nameInput.Focus()
			f.idInput.Blur()
		case fieldPath:
			f.field = fieldID
			f.idInput.Focus()
			f.pathInput.Blur()
		case fieldRecent:
			if f.recentIdx > 0 {
				f.recentIdx--
			} else {
				f.field = fieldPath
				f.pathInput.Focus()
			}
		case fieldAgent:
			// 找到目前 cursor 在哪個 group 的哪個位置，嘗試移到上一行
			row, col := f.agentRowCol()
			if row > 0 {
				// 移到上一行，保持 col 不超出
				f.agentCursor = f.cursorForRowCol(row-1, col)
			} else {
				// 已在 agent 最上方，跳到 recent 或 path
				if len(f.recentPaths) > 0 {
					f.field = fieldRecent
					f.recentIdx = len(f.recentPaths) - 1
				} else {
					f.field = fieldPath
					f.pathInput.Focus()
				}
			}
		}
		return m, nil

	case "down":
		switch f.field {
		case fieldHost:
			f.field = fieldName
			f.nameInput.Focus()
		case fieldName:
			f.field = fieldID
			f.nameInput.Blur()
			f.idInput.Focus()
		case fieldID:
			f.field = fieldPath
			f.idInput.Blur()
			f.pathInput.Focus()
		case fieldPath:
			f.pathInput.Blur()
			if len(f.recentPaths) > 0 {
				f.field = fieldRecent
				f.recentIdx = 0
			} else if f.totalAgentOptions() > 0 {
				f.field = fieldAgent
				f.agentCursor = 0
			}
		case fieldRecent:
			if f.recentIdx < len(f.recentPaths)-1 {
				f.recentIdx++
			} else if f.totalAgentOptions() > 0 {
				f.field = fieldAgent
				f.agentCursor = 0
			}
		case fieldAgent:
			row, col := f.agentRowCol()
			totalRows := f.totalAgentRows()
			if row < totalRows-1 {
				f.agentCursor = f.cursorForRowCol(row+1, col)
			}
		}
		return m, nil

	case "left":
		if f.field == fieldHost && f.hasHosts() && f.hostIdx > 0 {
			f.hostIdx--
			return m, nil
		}
		if f.field == fieldAgent && f.agentCursor > 0 {
			f.agentCursor--
		}
		return m, nil

	case "right":
		if f.field == fieldHost && f.hasHosts() && f.hostIdx < len(f.hosts)-1 {
			f.hostIdx++
			return m, nil
		}
		if f.field == fieldAgent && f.agentCursor < f.totalAgentOptions()-1 {
			f.agentCursor++
		}
		return m, nil

	case " ":
		if f.field == fieldRecent && len(f.recentPaths) > 0 {
			f.pathInput.SetValue(f.recentPaths[f.recentIdx])
			return m, nil
		}
		if f.field == fieldAgent {
			if f.agentCursor < len(f.agents) {
				f.selectedAgent = f.agentCursor
			} else {
				f.selectedAgent = -1 // Shell
			}
			return m, nil
		}
	}

	// 文字輸入轉發
	if f.field == fieldName {
		var cmd tea.Cmd
		f.nameInput, cmd = f.nameInput.Update(msg)
		return m, cmd
	}
	if f.field == fieldID {
		var cmd tea.Cmd
		f.idInput, cmd = f.idInput.Update(msg)
		return m, cmd
	}
	if f.field == fieldPath {
		var cmd tea.Cmd
		f.pathInput, cmd = f.pathInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// agentRowCol 回傳目前 agentCursor 在第幾行第幾列（行=群組 or Shell）。
func (f *newSessionForm) agentRowCol() (row, col int) {
	cursor := f.agentCursor
	for i, g := range f.agentGroups {
		if cursor < len(g.indices) {
			return i, cursor
		}
		cursor -= len(g.indices)
	}
	// Shell row
	return len(f.agentGroups), 0
}

// totalAgentRows 回傳 agent 區域的行數。
func (f *newSessionForm) totalAgentRows() int {
	rows := len(f.agentGroups)
	rows++ // Shell row
	return rows
}

// cursorForRowCol 回傳指定行列對應的全域 cursor。
func (f *newSessionForm) cursorForRowCol(row, col int) int {
	if row >= len(f.agentGroups) {
		// Shell row
		return len(f.agents)
	}
	g := f.agentGroups[row]
	if col >= len(g.indices) {
		col = len(g.indices) - 1
	}
	// 計算全域 cursor
	cursor := 0
	for i := 0; i < row; i++ {
		cursor += len(f.agentGroups[i].indices)
	}
	cursor += col
	return cursor
}

// renderNewSession 渲染 new session 表單。
func (m Model) renderNewSession() string {
	f := &m.newSession
	var b strings.Builder

	b.WriteString("\n")

	// 主機 tab 列（多主機模式才顯示）
	if f.hasHosts() {
		b.WriteString("  ")
		for i, h := range f.hosts {
			name := h.Name
			if i == f.hostIdx {
				// 選取的主機：使用主機顏色背景
				tabStyle := lipgloss.NewStyle().
					Background(lipgloss.Color(h.Color)).
					Foreground(lipgloss.Color("#ffffff")).
					Padding(0, 1).
					Bold(true)
				b.WriteString(tabStyle.Render(name))
			} else {
				b.WriteString(dimStyle.Render("  " + name + "  "))
			}
		}
		if f.field == fieldHost {
			b.WriteString("  " + dimStyle.Render("◄ ► 切換"))
		}
		b.WriteString("\n\n")
	}

	// 名稱（CustomName）
	nameMarker := "  "
	if f.field == fieldName {
		nameMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("  %s%s: %s\n",
		nameMarker, selectedStyle.Render("名稱"), f.nameInput.View()))

	// ID（tmux session name）
	idMarker := "  "
	if f.field == fieldID {
		idMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("  %s%s: %s\n",
		idMarker, selectedStyle.Render("ID"), f.idInput.View()))

	// 啟動路徑
	pathMarker := "  "
	if f.field == fieldPath {
		pathMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("  %s%s: %s\n",
		pathMarker, selectedStyle.Render("啟動路徑"), f.pathInput.View()))

	// 最近路徑
	for i, p := range f.recentPaths {
		marker := "    "
		pathStyle := dimStyle
		if f.field == fieldRecent && i == f.recentIdx {
			marker = "  " + selectedStyle.Render("► ")
			pathStyle = lipgloss.NewStyle()
		}
		b.WriteString(fmt.Sprintf("%s%s\n", marker, pathStyle.Render(p)))
	}

	// 啟動 Agent 區域
	b.WriteString(fmt.Sprintf("\n  %s\n", selectedStyle.Render("啟動 Agent")))

	globalIdx := 0
	for _, g := range f.agentGroups {
		// 群組標題（左側）
		label := "         " // 9 chars padding for no-title groups
		if g.title != "" {
			label = fmt.Sprintf("  %-7s", g.title)
		}

		// 選項（右側橫排）
		var opts []string
		for _, agentIdx := range g.indices {
			radio := "○"
			if globalIdx == f.selectedAgent {
				radio = "●"
			}
			name := f.agents[agentIdx].Name
			optStr := fmt.Sprintf("%s %s", radio, name)
			if f.field == fieldAgent && globalIdx == f.agentCursor {
				optStr = selectedStyle.Render(optStr)
			}
			opts = append(opts, optStr)
			globalIdx++
		}
		b.WriteString(fmt.Sprintf("%s%s\n", label, strings.Join(opts, "   ")))
	}

	// Shell 選項（始終在最後）
	shellRadio := "○"
	if f.selectedAgent == -1 {
		shellRadio = "●"
	}
	shellLabel := fmt.Sprintf("%s Shell", shellRadio)
	if f.field == fieldAgent && f.agentCursor == len(f.agents) {
		shellLabel = selectedStyle.Render(shellLabel)
	}
	b.WriteString(fmt.Sprintf("         %s\n", shellLabel))

	// 操作提示
	b.WriteString(fmt.Sprintf("\n  %s\n",
		dimStyle.Render("[↑↓] 移動  [←→] 切換  [space] 選取  [Enter] 建立  [Esc] 取消")))

	return b.String()
}
