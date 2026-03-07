package setup

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
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
)

// InstallMode 代表安裝模式（與 config.InstallMode 對應）。
type InstallMode int

const (
	ModeFull   InstallMode = iota // 完整模式
	ModeClient                    // 純客戶端模式
)

// Component 代表一個可安裝/卸載的元件。
type Component struct {
	Label       string
	InstallFn   func() (string, error)
	UninstallFn func() (string, error)
	Checked     bool   // 初始勾選狀態
	Disabled    bool   // 禁用：不可切換/安裝
	Note        string // 附加資訊，顯示在 Label 下方
	FullOnly    bool   // 僅完整模式安裝
	Installed   bool   // 偵測到已安裝
}

// result 記錄單一元件的安裝結果。
type result struct {
	label   string
	message string
	err     error
}

// phase 表示 TUI 的狀態階段。
type phase int

const (
	phaseSelect  phase = iota // 選擇元件
	phaseRunning              // 執行中
	phaseDone                 // 完成
)

// Model 是 setup TUI 的 Bubble Tea model。
type Model struct {
	components  []Component
	checked     []bool
	cursor      int
	phase       phase
	results     []result
	quitting    bool
	daemonHint  string       // 安裝完成後顯示的 daemon 提示
	restartFn   func() error // 重啟 daemon 的 callback
	restarted   bool         // 是否已執行重啟
	restartErr  error        // 重啟結果
	installMode InstallMode  // 安裝模式
}

// NewModel 建立 setup TUI model，根據各元件的 Checked 欄位初始化勾選狀態。
func NewModel(components []Component) Model {
	checked := make([]bool, len(components))
	for i, comp := range components {
		checked[i] = comp.Checked
	}
	return Model{
		components: components,
		checked:    checked,
	}
}

// SetInstallMode 設定安裝模式，並更新元件勾選狀態。
func (m *Model) SetInstallMode(mode InstallMode) {
	m.installMode = mode
	m.applyMode()
}

// InstallMode 回傳當前安裝模式。
func (m Model) InstallMode() InstallMode {
	return m.installMode
}

// applyMode 根據當前模式更新元件的勾選與禁用狀態。
func (m *Model) applyMode() {
	for i, comp := range m.components {
		if comp.FullOnly {
			if m.installMode == ModeClient {
				m.checked[i] = false
			} else {
				m.checked[i] = comp.Checked
			}
		}
	}
}

// SetDaemonHint 設定安裝完成後顯示的 daemon 提示訊息。
func (m *Model) SetDaemonHint(hint string) {
	m.daemonHint = hint
}

// SetRestartFn 設定重啟 daemon 的 callback。
func (m *Model) SetRestartFn(fn func() error) {
	m.restartFn = fn
}

func (m Model) Init() tea.Cmd {
	return nil
}

// installMsg 用於觸發安裝流程的訊息。
type installMsg struct{}

// resultMsg 回報安裝結果的訊息。
type resultMsg struct {
	results []result
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case installMsg:
		return m.runInstall()
	case resultMsg:
		m.results = msg.results
		m.phase = phaseDone
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.phase != phaseSelect {
		if m.phase == phaseDone {
			// 有 restartFn 且尚未重啟：等待 Y/n 回應
			if m.daemonHint != "" && m.restartFn != nil && !m.restarted {
				switch msg.String() {
				case "y", "Y", "enter":
					m.restartErr = m.restartFn()
					m.restarted = true
					m.quitting = true
					return m, tea.Quit
				case "n", "N", "esc", "ctrl+c":
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			}
			// 無 restartFn：任意鍵退出
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.components)-1 {
			m.cursor++
		}
	case "left", "right":
		if m.installMode == ModeFull {
			m.installMode = ModeClient
		} else {
			m.installMode = ModeFull
		}
		m.applyMode()
		return m, nil
	case " ":
		if m.cursor < len(m.checked) {
			comp := m.components[m.cursor]
			disabled := comp.Disabled || (comp.FullOnly && m.installMode == ModeClient)
			if !disabled {
				m.checked[m.cursor] = !m.checked[m.cursor]
			}
		}
	case "enter":
		m.phase = phaseRunning
		return m, func() tea.Msg { return installMsg{} }
	}
	return m, nil
}

func (m Model) runInstall() (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		var results []result
		for i, comp := range m.components {
			if comp.Disabled {
				continue
			}

			isClientFullOnly := comp.FullOnly && m.installMode == ModeClient
			if isClientFullOnly && comp.Installed && comp.UninstallFn != nil {
				// [-] 已安裝的 FullOnly 元件在 client mode 執行移除
				msg, err := comp.UninstallFn()
				results = append(results, result{label: comp.Label, message: msg, err: err})
				continue
			}

			if !m.checked[i] || isClientFullOnly {
				continue
			}
			msg, err := comp.InstallFn()
			results = append(results, result{label: comp.Label, message: msg, err: err})
		}
		return resultMsg{results: results}
	}
}

func (m Model) View() string {
	if m.quitting && !m.restarted {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(headerStyle.Render("tsm setup"))
	b.WriteString(dimStyle.Render("  " + version.String()))
	b.WriteString("\n\n")

	switch m.phase {
	case phaseSelect:
		// 模式選擇器 — 水平 tab
		tabs := []string{"完整模式", "純客戶端"}
		b.WriteString("  ")
		for i, tab := range tabs {
			if i > 0 {
				b.WriteString(dimStyle.Render(" │ "))
			}
			if i == int(m.installMode) {
				b.WriteString(selectedStyle.Render(tab))
			} else {
				b.WriteString(dimStyle.Render(tab))
			}
		}
		b.WriteString("    ")
		b.WriteString(dimStyle.Render("← → 切換"))
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("選擇要安裝的元件 (Space 切換, Enter 確認, q 取消)"))
		b.WriteString("\n\n")
		for i, comp := range m.components {
			cursor := "  "
			if i == m.cursor {
				cursor = selectedStyle.Render("► ")
			}

			if comp.Disabled {
				label := errorStyle.Render(comp.Label)
				b.WriteString(fmt.Sprintf("%s[-] %s\n", cursor, label))
				if comp.Note != "" {
					b.WriteString(fmt.Sprintf("       %s\n", errorStyle.Render(comp.Note)))
				}
				continue
			}

			isClientFullOnly := comp.FullOnly && m.installMode == ModeClient
			if isClientFullOnly {
				if comp.Installed {
					// [-] 已安裝，將移除
					label := warnStyle.Render(comp.Label)
					b.WriteString(fmt.Sprintf("%s[-] %s\n", cursor, label))
					if comp.Note != "" {
						b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(comp.Note)))
					}
				} else {
					// [ ] 未安裝，不適用
					label := dimStyle.Render(comp.Label)
					b.WriteString(fmt.Sprintf("%s[ ] %s\n", cursor, label))
				}
				continue
			}

			check := "[ ]"
			if m.checked[i] {
				check = "[x]"
			}
			label := comp.Label
			if i == m.cursor {
				label = selectedStyle.Render(label)
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, check, label))
			if comp.Note != "" {
				b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(comp.Note)))
			}
		}

	case phaseRunning:
		b.WriteString("安裝中...\n")

	case phaseDone:
		for _, r := range m.results {
			if r.err != nil {
				b.WriteString(errorStyle.Render(fmt.Sprintf("✗ %s: %v", r.label, r.err)))
			} else {
				b.WriteString(successStyle.Render(fmt.Sprintf("✓ %s: %s", r.label, r.message)))
			}
			b.WriteString("\n")
		}
		if m.daemonHint != "" {
			b.WriteString("\n")
			if m.restarted {
				if m.restartErr != nil {
					b.WriteString(errorStyle.Render(fmt.Sprintf("✗ daemon 重啟失敗: %v", m.restartErr)))
				} else {
					b.WriteString(successStyle.Render("✓ daemon 已重新啟動"))
				}
				b.WriteString("\n")
			} else if m.restartFn != nil {
				b.WriteString(warnStyle.Render("⚠ daemon 需要重新啟動"))
				b.WriteString("  ")
				b.WriteString(dimStyle.Render("重新啟動？(Y/n)"))
				b.WriteString("\n")
			} else {
				b.WriteString(warnStyle.Render("⚠ " + m.daemonHint))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// Checked 回傳各元件的勾選狀態（用於測試）。
func (m Model) Checked() []bool {
	out := make([]bool, len(m.checked))
	copy(out, m.checked)
	return out
}

// Phase 回傳當前階段（用於測試）。
func (m Model) Phase() phase {
	return m.phase
}

// Results 回傳執行結果（用於測試）。
func (m Model) Results() []result {
	return m.results
}

// RunUninstallAll 依序執行所有元件的 UninstallFn，不需 TUI。
func RunUninstallAll(components []Component) {
	for _, comp := range components {
		msg, err := comp.UninstallFn()
		if err != nil {
			fmt.Printf("✗ %s: %v\n", comp.Label, err)
		} else {
			fmt.Printf("✓ %s: %s\n", comp.Label, msg)
		}
	}
}
