// Package cfgtui 提供雙分頁的 Config 設定 TUI（Bubble Tea model）。
package cfgtui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/version"
)

// Tab 代表分頁索引。
type Tab int

const (
	TabClient Tab = iota // 客戶端分頁
	TabServer            // 伺服器分頁
)

// 樣式定義（Tokyo Night 色系，與 setup.go 一致）。
var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#787fa0"))

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#1a1b26")).
			Background(lipgloss.Color("#7aa2f7")).
			Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#787fa0")).
				Background(lipgloss.Color("#24283b")).
				Padding(0, 1)

	labelStyle = lipgloss.NewStyle().Width(22)
)

// field 代表一個可編輯的設定欄位。
type field struct {
	key   string // 設定欄位名稱，如 "local.bar_bg"
	label string // 欄位說明
	value string // 目前值
}

// clientFields 回傳客戶端分頁的欄位定義。
func clientFields(cfg config.Config) []field {
	return []field{
		{key: "local.bar_bg", label: "本地 bar 背景色", value: cfg.Local.BarBG},
		{key: "local.badge_bg", label: "本地 badge 背景色", value: cfg.Local.BadgeBG},
		{key: "local.badge_fg", label: "本地 badge 前景色", value: cfg.Local.BadgeFG},
		{key: "remote.bar_bg", label: "遠端 bar 背景色", value: cfg.Remote.BarBG},
		{key: "remote.badge_bg", label: "遠端 badge 背景色", value: cfg.Remote.BadgeBG},
		{key: "remote.badge_fg", label: "遠端 badge 前景色", value: cfg.Remote.BadgeFG},
	}
}

// serverFields 回傳伺服器分頁的欄位定義。
func serverFields(cfg config.Config) []field {
	return []field{
		{key: "preview_lines", label: "預覽行數", value: strconv.Itoa(cfg.PreviewLines)},
		{key: "poll_interval_sec", label: "輪詢間隔", value: strconv.Itoa(cfg.PollIntervalSec)},
		{key: "local.bar_bg", label: "本地 bar 背景色", value: cfg.Local.BarBG},
		{key: "local.badge_bg", label: "本地 badge 背景色", value: cfg.Local.BadgeBG},
		{key: "local.badge_fg", label: "本地 badge 前景色", value: cfg.Local.BadgeFG},
		{key: "remote.bar_bg", label: "遠端 bar 背景色", value: cfg.Remote.BarBG},
		{key: "remote.badge_bg", label: "遠端 badge 背景色", value: cfg.Remote.BadgeBG},
		{key: "remote.badge_fg", label: "遠端 badge 前景色", value: cfg.Remote.BadgeFG},
	}
}

// defaultFieldValue 回傳指定欄位的預設值。
func defaultFieldValue(key string) string {
	def := config.Default()
	switch key {
	case "preview_lines":
		return strconv.Itoa(def.PreviewLines)
	case "poll_interval_sec":
		return strconv.Itoa(def.PollIntervalSec)
	case "local.bar_bg":
		return def.Local.BarBG
	case "local.badge_bg":
		return def.Local.BadgeBG
	case "local.badge_fg":
		return def.Local.BadgeFG
	case "remote.bar_bg":
		return def.Remote.BarBG
	case "remote.badge_bg":
		return def.Remote.BadgeBG
	case "remote.badge_fg":
		return def.Remote.BadgeFG
	default:
		return ""
	}
}

// Model 是 Config TUI 的 Bubble Tea model。
type Model struct {
	cfg      config.Config // 工作中的設定（隨編輯更新）
	tab      Tab
	cursor   int
	editing  bool
	saved    bool
	quitting bool
	hostname string

	// 各分頁的欄位快照
	clientFields []field
	serverFields []field

	// 文字輸入元件（編輯模式使用）
	textInput textinput.Model
}

// NewModel 建立 Config TUI model。
func NewModel(cfg config.Config) Model {
	ti := textinput.New()
	ti.CharLimit = 64

	return Model{
		cfg:          cfg,
		tab:          TabClient,
		clientFields: clientFields(cfg),
		serverFields: serverFields(cfg),
		textInput:    ti,
	}
}

// Tab 回傳當前分頁。
func (m Model) Tab() Tab {
	return m.tab
}

// Cursor 回傳當前游標位置。
func (m Model) Cursor() int {
	return m.cursor
}

// Editing 回傳是否在編輯模式。
func (m Model) Editing() bool {
	return m.editing
}

// Saved 回傳是否已儲存。
func (m Model) Saved() bool {
	return m.saved
}

// SetHostname 設定顯示的主機名稱。
func (m *Model) SetHostname(hostname string) {
	m.hostname = hostname
}

// FieldCount 回傳指定分頁的欄位數量。
func (m Model) FieldCount(tab Tab) int {
	switch tab {
	case TabClient:
		return len(m.clientFields)
	case TabServer:
		return len(m.serverFields)
	default:
		return 0
	}
}

// currentFields 回傳當前分頁的欄位。
func (m Model) currentFields() []field {
	if m.tab == TabServer {
		return m.serverFields
	}
	return m.clientFields
}

// setCurrentField 設定當前分頁指定位置的欄位值。
func (m *Model) setCurrentField(idx int, value string) {
	if m.tab == TabServer {
		m.serverFields[idx].value = value
	} else {
		m.clientFields[idx].value = value
	}
	// 同步到另一分頁的共享欄位
	m.syncFields()
}

// syncFields 同步兩個分頁之間的共享欄位。
func (m *Model) syncFields() {
	// 建立 key→value 的 map，以當前分頁為準
	values := make(map[string]string)
	for _, f := range m.clientFields {
		values[f.key] = f.value
	}
	for _, f := range m.serverFields {
		values[f.key] = f.value
	}

	// 以當前分頁的值覆蓋對方
	src := m.currentFields()
	for _, f := range src {
		values[f.key] = f.value
	}

	// 回寫到兩個分頁
	for i, f := range m.clientFields {
		if v, ok := values[f.key]; ok {
			m.clientFields[i].value = v
		}
	}
	for i, f := range m.serverFields {
		if v, ok := values[f.key]; ok {
			m.serverFields[i].value = v
		}
	}
}

// ResultConfig 回傳編輯後的 Config。
func (m Model) ResultConfig() config.Config {
	cfg := m.cfg

	// 從 serverFields 取得所有欄位值（包含 client 共享欄位）
	values := make(map[string]string)
	for _, f := range m.serverFields {
		values[f.key] = f.value
	}
	// clientFields 也更新（若有獨佔欄位）
	for _, f := range m.clientFields {
		values[f.key] = f.value
	}

	if v, ok := values["preview_lines"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.PreviewLines = n
		}
	}
	if v, ok := values["poll_interval_sec"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.PollIntervalSec = n
		}
	}
	if v, ok := values["local.bar_bg"]; ok {
		cfg.Local.BarBG = v
	}
	if v, ok := values["local.badge_bg"]; ok {
		cfg.Local.BadgeBG = v
	}
	if v, ok := values["local.badge_fg"]; ok {
		cfg.Local.BadgeFG = v
	}
	if v, ok := values["remote.bar_bg"]; ok {
		cfg.Remote.BarBG = v
	}
	if v, ok := values["remote.badge_bg"]; ok {
		cfg.Remote.BadgeBG = v
	}
	if v, ok := values["remote.badge_fg"]; ok {
		cfg.Remote.BadgeFG = v
	}

	return cfg
}

// Init 實作 tea.Model 介面。
func (m Model) Init() tea.Cmd {
	return nil
}

// Update 實作 tea.Model 介面。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey 處理鍵盤事件。
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// 編輯模式下的按鍵處理
	if m.editing {
		return m.handleEditKey(msg)
	}

	// 一般模式下的按鍵處理
	switch msg.String() {
	case "ctrl+s":
		m.saved = true
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "left":
		if m.tab > TabClient {
			m.tab--
			m.cursor = 0
		}
	case "right":
		if m.tab < TabServer {
			m.tab++
			m.cursor = 0
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		fields := m.currentFields()
		if m.cursor < len(fields)-1 {
			m.cursor++
		}
	case "enter":
		m.editing = true
		fields := m.currentFields()
		if m.cursor < len(fields) {
			m.textInput.SetValue(fields[m.cursor].value)
			m.textInput.Focus()
			m.textInput.CursorEnd()
		}
	case "delete", "backspace":
		fields := m.currentFields()
		if m.cursor < len(fields) {
			defVal := defaultFieldValue(fields[m.cursor].key)
			m.setCurrentField(m.cursor, defVal)
		}
	}
	return m, nil
}

// handleEditKey 處理編輯模式下的按鍵。
func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		// 取消編輯，不更新值
		m.editing = false
		m.textInput.Blur()
		return m, nil
	case tea.KeyEnter:
		// 確認編輯，更新值
		m.editing = false
		newValue := m.textInput.Value()
		m.setCurrentField(m.cursor, newValue)
		m.textInput.Blur()
		return m, nil
	default:
		// 委託給 textinput 處理
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// View 實作 tea.Model 介面。
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	title := "tsm config"
	if m.hostname != "" {
		title += " @ " + m.hostname
	}
	b.WriteString(headerStyle.Render(title))
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString("\n\n")

	// Tab 選擇器
	tabs := []string{"客戶端", "伺服器"}
	b.WriteString("  ")
	for i, tab := range tabs {
		if i > 0 {
			b.WriteString(" ")
		}
		if Tab(i) == m.tab {
			b.WriteString(activeTabStyle.Render(tab))
		} else {
			b.WriteString(inactiveTabStyle.Render(tab))
		}
	}
	b.WriteString("\n\n")

	// 欄位列表
	fields := m.currentFields()
	for i, f := range fields {
		cursor := "  "
		if i == m.cursor {
			cursor = selectedStyle.Render("► ")
		}

		key := labelStyle.Render(f.key)
		var valueStr string
		if m.editing && i == m.cursor {
			valueStr = m.textInput.View()
		} else {
			defVal := defaultFieldValue(f.key)
			if f.value == defVal {
				valueStr = dimStyle.Render(f.value)
			} else if f.value == "" {
				valueStr = dimStyle.Render("(空)")
			} else {
				valueStr = f.value
			}
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, key, valueStr))
	}

	// 底部操作提示
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Enter 編輯 | Del 重置 | Ctrl+S 儲存 | q/Esc 取消"))
	b.WriteString("\n")

	// 左側 1 space padding
	output := b.String()
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = " " + line
		}
	}
	return strings.Join(lines, "\n")
}
