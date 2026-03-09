package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/config"
)

// newSessionField 代表表單中的焦點區塊。
type newSessionField int

const (
	fieldName   newSessionField = iota // Session 名稱
	fieldPath                          // 啟動路徑
	fieldRecent                        // 最近路徑選擇
	fieldAgent                         // Agent 選擇
)

// agentGroup 是分群後的 agent 清單。
type agentGroup struct {
	title   string
	indices []int // 對應 agents 切片的 index
}

// newSessionForm 管理 new session 表單的狀態。
type newSessionForm struct {
	nameInput textinput.Model
	pathInput textinput.Model

	field       newSessionField // 目前焦點區塊
	recentPaths []string        // 最近路徑
	recentIdx   int             // 最近路徑的游標

	agents        []config.AgentEntry // 已啟用的 agent 清單
	agentGroups   []agentGroup        // 分群後的 agent
	agentCursor   int                 // agent 選項中的全域游標（含 Shell）
	selectedAgent int                 // 已選取的 agent index（-1 = Shell）
}

func (f *newSessionForm) totalAgentOptions() int {
	return len(f.agents) + 1 // +1 for Shell
}

// initForm 初始化表單。
func (f *newSessionForm) initForm(agents []config.AgentEntry, recentPaths []string) {
	f.nameInput = textinput.New()
	f.nameInput.Placeholder = "session-name"
	f.nameInput.Focus()

	f.pathInput = textinput.New()
	f.pathInput.Placeholder = "~/Workspace/..."

	f.field = fieldName
	f.recentPaths = recentPaths
	f.recentIdx = 0
	f.selectedAgent = -1 // Shell selected by default
	f.agentCursor = 0

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

// updateNewSession 處理 ModeNewSession 的按鍵。
func (m Model) updateNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.newSession
	key := msg.String()

	switch key {
	case "esc":
		m.mode = ModeNormal
		return m, nil

	case "enter":
		name := f.nameInput.Value()
		if name == "" {
			return m, nil
		}
		path := f.pathInput.Value()
		command := ""
		if f.selectedAgent >= 0 && f.selectedAgent < len(f.agents) {
			command = f.agents[f.selectedAgent].Command
		}

		// 建立 session
		if c := m.clientForCursor(); c != nil {
			if err := c.CreateSession(context.Background(), name, path, command); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
		} else if m.deps.TmuxMgr != nil {
			if err := m.deps.TmuxMgr.NewSession(name, path); err != nil {
				m.err = err
				m.mode = ModeNormal
				return m, nil
			}
			if command != "" {
				_ = m.deps.TmuxMgr.SendKeys(name, command)
			}
		}

		// 記錄路徑歷史
		if path != "" && m.deps.Store != nil {
			_ = m.deps.Store.AddPathHistory(path)
		}

		m.mode = ModeNormal
		return m, loadSessionsCmd(m.deps)

	case "up":
		switch f.field {
		case fieldName:
			// 已在最頂部
		case fieldPath:
			f.field = fieldName
			f.nameInput.Focus()
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
		case fieldName:
			f.field = fieldPath
			f.nameInput.Blur()
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
		if f.field == fieldAgent && f.agentCursor > 0 {
			f.agentCursor--
		}
		return m, nil

	case "right":
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

	// Session 名稱
	nameMarker := "  "
	if f.field == fieldName {
		nameMarker = selectedStyle.Render("► ")
	}
	b.WriteString(fmt.Sprintf("\n  %s%s: %s\n",
		nameMarker, selectedStyle.Render("Session 名稱"), f.nameInput.View()))

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
