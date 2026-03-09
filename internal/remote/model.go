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

// CountdownTickMsg 倒數計時的每秒 tick。
type CountdownTickMsg struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

func countdownTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return CountdownTickMsg{}
	})
}

// ReconnectModel 是重連 modal 的 Bubble Tea model。
type ReconnectModel struct {
	host       string
	session    string
	state      ReconnState
	attempt    int
	frame      int
	countdown  int
	quit       bool
	backToMenu bool
	width      int
	height     int
}

// NewReconnectModel 建立新的重連 modal model。
func NewReconnectModel(host, session string) ReconnectModel {
	return ReconnectModel{
		host:      host,
		session:   session,
		state:     StateDisconnected,
		countdown: CountdownSec,
	}
}

// State 回傳目前重連狀態。
func (m ReconnectModel) State() ReconnState { return m.state }

// Attempt 回傳目前重連嘗試次數。
func (m ReconnectModel) Attempt() int { return m.attempt }

// Quit 回傳使用者是否選擇退出。
func (m ReconnectModel) Quit() bool { return m.quit }

// Countdown 回傳目前倒數秒數。
func (m ReconnectModel) Countdown() int { return m.countdown }

// BackToMenu 回傳是否應回到選單。
func (m ReconnectModel) BackToMenu() bool { return m.backToMenu }

// Init 實作 tea.Model 介面，啟動倒數計時。
func (m ReconnectModel) Init() tea.Cmd { return countdownTickCmd() }

// Update 處理訊息更新。
func (m ReconnectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case CountdownTickMsg:
		if m.countdown > 1 {
			m.countdown--
			return m, countdownTickCmd()
		}
		if m.countdown == 1 {
			m.countdown = 0
			m.state = StateAskContinue
			return m, nil
		}
		return m, nil
	case tea.KeyMsg:
		// StateAskContinue 時優先處理 Y/N
		if m.state == StateAskContinue {
			switch msg.Type {
			case tea.KeyEscape:
				m.backToMenu = true
				return m, tea.Quit
			case tea.KeyRunes:
				switch string(msg.Runes) {
				case "y", "Y":
					m.countdown = CountdownSec
					m.state = StateConnecting
					return m, countdownTickCmd()
				case "n", "N":
					m.quit = true
					return m, tea.Quit
				case "q":
					m.quit = true
					return m, tea.Quit
				}
			case tea.KeyCtrlC:
				m.quit = true
				return m, tea.Quit
			}
			return m, nil
		}
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
		m.attempt = msg.Attempt
		// StateConnected 優先處理：無論當前狀態都應結束
		if msg.State == StateConnected {
			m.state = StateConnected
			return m, tea.Quit
		}
		// StateAskContinue 時忽略 StateConnecting，避免輪詢覆蓋使用者提示
		if m.state == StateAskContinue && msg.State == StateConnecting {
			return m, nil
		}
		m.state = msg.State
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

// View 渲染重連 modal 畫面（1/2 寬高的 popup）。
func (m ReconnectModel) View() string {
	if m.quit || m.backToMenu {
		return ""
	}

	var icon, status string
	iconDisconnected := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555"))
	iconConnecting := lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c"))
	iconConnected := lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4"))

	switch m.state {
	case StateDisconnected:
		icon = iconDisconnected.Render("✗")
		status = fmt.Sprintf("連線中斷  %s", m.host)
	case StateConnecting:
		frames := []string{"◐", "◓", "◑", "◒"}
		icon = iconConnecting.Render(frames[m.frame%len(frames)])
		status = fmt.Sprintf("重新連線中...  %d 秒", m.countdown)
	case StateConnected:
		icon = iconConnected.Render("●")
		status = "已連線"
	case StateAskContinue:
		icon = iconDisconnected.Render("✗")
		status = "連線逾時"
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s %s", icon, status))
	lines = append(lines, "")

	if m.state == StateAskContinue {
		lines = append(lines, "  是否繼續嘗試連線？")
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("  [Y] 繼續  [N] 退出  [Esc] 回到選單"))
	} else {
		lines = append(lines, dimStyle.Render("  [Esc] 回到選單  [q] 退出"))
	}
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	if m.width > 0 && m.height > 0 {
		popupW := m.width / 2
		popupH := m.height / 2
		bordered := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6272a4")).
			Width(popupW).
			Height(popupH).
			Render(content)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, bordered)
	}
	return content
}
