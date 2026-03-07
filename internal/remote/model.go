package remote

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ReconnStateMsg 從重連 goroutine 回報狀態變更。
type ReconnStateMsg struct {
	State   ReconnState
	Attempt int
}

// ReconnSessionGoneMsg 表示目標 session 已不存在。
type ReconnSessionGoneMsg struct{}

// AnimTickMsg 用於重連動畫的 tick 訊息。
type AnimTickMsg struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// ReconnectModel 是重連 modal 的 Bubble Tea model。
type ReconnectModel struct {
	host       string
	session    string
	state      ReconnState
	attempt    int
	frame      int
	quit       bool
	backToMenu bool
	width      int
	height     int
}

// NewReconnectModel 建立新的重連 modal model。
func NewReconnectModel(host, session string) ReconnectModel {
	return ReconnectModel{
		host:    host,
		session: session,
		state:   StateDisconnected,
	}
}

// State 回傳目前重連狀態。
func (m ReconnectModel) State() ReconnState { return m.state }

// Attempt 回傳目前重連嘗試次數。
func (m ReconnectModel) Attempt() int { return m.attempt }

// Quit 回傳使用者是否選擇退出。
func (m ReconnectModel) Quit() bool { return m.quit }

// BackToMenu 回傳是否應回到選單。
func (m ReconnectModel) BackToMenu() bool { return m.backToMenu }

// Init 實作 tea.Model 介面。
func (m ReconnectModel) Init() tea.Cmd { return nil }

// Update 處理訊息更新。
func (m ReconnectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEscape:
			m.backToMenu = true
			return m, tea.Quit
		case tea.KeyRunes:
			if string(msg.Runes) == "q" {
				m.quit = true
				return m, tea.Quit
			}
		case tea.KeyCtrlC:
			m.quit = true
			return m, tea.Quit
		}
	case ReconnStateMsg:
		m.state = msg.State
		m.attempt = msg.Attempt
		if m.state == StateConnected {
			return m, tea.Quit
		}
		if m.state == StateConnecting {
			return m, animTickCmd()
		}
		return m, nil
	case ReconnSessionGoneMsg:
		m.backToMenu = true
		return m, tea.Quit
	case AnimTickMsg:
		m.frame++
		return m, animTickCmd()
	}
	return m, nil
}

// View 渲染重連 modal 畫面。
func (m ReconnectModel) View() string {
	if m.quit || m.backToMenu {
		return ""
	}

	var icon, status string
	iconDisconnected := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	iconConnecting := lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c"))
	iconConnected := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))

	switch m.state {
	case StateDisconnected:
		icon = iconDisconnected.Render("✗")
		status = fmt.Sprintf("連線中斷  %s", m.host)
	case StateConnecting:
		frames := []string{"◐", "◓", "◑", "◒"}
		icon = iconConnecting.Render(frames[m.frame%len(frames)])
		status = fmt.Sprintf("重新連線中... (第 %d 次)", m.attempt)
	case StateConnected:
		icon = iconConnected.Render("●")
		status = "已連線"
	}

	line1 := fmt.Sprintf("  %s %s", icon, status)
	line2 := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")).Render(
		"  [Esc] 回到選單  [q] 退出")

	box := strings.Join([]string{"", line1, "", line2, ""}, "\n")

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}
	return box
}
