package launcher

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/wake/tmux-session-menu/internal/version"
)

// 樣式定義（從 ui/styles.go 複製色碼，避免引入 ui 套件的重依賴）。
var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c0caf5"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#787fa0"))
)

// Choice 代表使用者的選擇。
type Choice int

const (
	ChoiceNone      Choice = iota // 未選擇（退出或尚未選擇）
	ChoiceRemote                  // 連線遠端
	ChoiceLocal                   // 簡易本地端
	ChoiceFullSetup               // 完整設定
)

var tabLabels = []string{"連線遠端", "簡易本地端", "完整設定"}

// Model 是 launcher TUI 的 Bubble Tea model。
type Model struct {
	activeTab   int
	hostInput   string
	placeholder string
	choice      Choice
	quitting    bool
}

// NewModel 建立 launcher model，placeholder 為上次使用的遠端主機。
func NewModel(placeholder string) Model {
	return Model{placeholder: placeholder}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "left":
		if m.activeTab > 0 {
			m.activeTab--
		}
	case "right":
		if m.activeTab < len(tabLabels)-1 {
			m.activeTab++
		}
	case "enter":
		return m.handleEnter()
	case "tab":
		if m.activeTab == 0 && m.hostInput == "" && m.placeholder != "" {
			m.hostInput = m.placeholder
		}
	case "backspace":
		if m.activeTab == 0 && len(m.hostInput) > 0 {
			m.hostInput = m.hostInput[:len(m.hostInput)-1]
		}
	default:
		if m.activeTab == 0 && msg.Type == tea.KeyRunes {
			m.hostInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case 0: // 連線遠端
		if m.hostInput == "" {
			return m, nil
		}
		m.choice = ChoiceRemote
	case 1: // 簡易本地端
		m.choice = ChoiceLocal
	case 2: // 完整設定
		m.choice = ChoiceFullSetup
	}
	return m, tea.Quit
}

func (m Model) View() string {
	if m.quitting && m.choice == ChoiceNone {
		return ""
	}

	var b strings.Builder

	// 標題列
	b.WriteString(headerStyle.Render("tsm"))
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString("\n\n")

	// Tab bar
	b.WriteString("  ")
	for i, tab := range tabLabels {
		if i > 0 {
			b.WriteString(dimStyle.Render(" │ "))
		}
		if i == m.activeTab {
			b.WriteString(selectedStyle.Render(tab))
		} else {
			b.WriteString(dimStyle.Render(tab))
		}
	}
	b.WriteString("\n\n")

	// Tab 內容
	switch m.activeTab {
	case 0:
		b.WriteString("  SSH 主機: ")
		if m.hostInput == "" && m.placeholder != "" {
			b.WriteString(dimStyle.Render(m.placeholder))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("            Tab 自動填入, Enter 連線"))
		} else if m.hostInput == "" {
			b.WriteString(dimStyle.Render("輸入主機名稱..."))
		} else {
			b.WriteString(fmt.Sprintf("%s█", m.hostInput))
		}
	case 1:
		b.WriteString(dimStyle.Render("  直接啟動本地 tmux 管理介面（不啟用 daemon）"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Enter 啟動"))
	case 2:
		b.WriteString(dimStyle.Render("  安裝完整元件後啟動（hook + bind key + daemon）"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("  Enter 開始設定"))
	}
	b.WriteString("\n")

	return b.String()
}

// ActiveTab 回傳目前選取的 tab 索引。
func (m Model) ActiveTab() int { return m.activeTab }

// Choice 回傳使用者的選擇。
func (m Model) Choice() Choice { return m.choice }

// Host 回傳使用者輸入的遠端主機名。
func (m Model) Host() string { return m.hostInput }

// Placeholder 回傳 placeholder 主機名。
func (m Model) Placeholder() string { return m.placeholder }
