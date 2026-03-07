# tsm --remote Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `tsm --remote <host>` to connect to a remote tsm-daemon via SSH tunnel, with auto-reconnect on disconnect.

**Architecture:** SSH unix socket forwarding tunnels gRPC traffic to the remote daemon. Session picker reuses existing `ui.Model`. Attach spawns `ssh -t host tmux attach`. Disconnect triggers a Bubble Tea reconnect modal with exponential backoff.

**Tech Stack:** Go 1.24, `os/exec` for SSH, `charmbracelet/bubbletea` for reconnect modal, existing gRPC client.

---

### Task 1: DialSocket in client.go

**Files:**
- Modify: `internal/client/client.go`
- Test: `internal/client/client_test.go`

**Step 1: Write the failing test**

In `internal/client/client_test.go`, add:

```go
func TestClient_DialSocket_InvalidPath(t *testing.T) {
	_, err := DialSocket("/nonexistent/path/tsm.sock")
	assert.Error(t, err)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestClient_DialSocket -count=1 -v`
Expected: FAIL — `DialSocket` undefined

**Step 3: Write minimal implementation**

In `internal/client/client.go`, add after `Dial`:

```go
// DialSocket 連線到指定的 unix socket（用於 remote 模式，不觸發 auto-start daemon）。
func DialSocket(sockPath string) (*Client, error) {
	conn, err := tryConnect(sockPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		rpc:  tsmv1.NewSessionManagerClient(conn),
	}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/client/ -run TestClient_DialSocket -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/client/client.go internal/client/client_test.go
git commit -m "feat(client): add DialSocket for remote mode"
```

---

### Task 2: Tunnel — LocalSocketPath + SSH args

**Files:**
- Create: `internal/remote/tunnel.go`
- Create: `internal/remote/tunnel_test.go`

**Step 1: Write the failing tests**

Create `internal/remote/tunnel_test.go`:

```go
package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalSocketPath_Deterministic(t *testing.T) {
	a := LocalSocketPath("myhost")
	b := LocalSocketPath("myhost")
	assert.Equal(t, a, b, "same host should produce same path")
}

func TestLocalSocketPath_DifferentHosts(t *testing.T) {
	a := LocalSocketPath("host-a")
	b := LocalSocketPath("host-b")
	assert.NotEqual(t, a, b, "different hosts should produce different paths")
}

func TestLocalSocketPath_Format(t *testing.T) {
	p := LocalSocketPath("myhost")
	assert.True(t, strings.HasPrefix(p, "/"), "should be absolute path")
	assert.True(t, strings.HasSuffix(p, ".sock"), "should end with .sock")
	assert.True(t, strings.Contains(p, "tsm-"), "should contain tsm- prefix")
}

func TestTunnelArgs(t *testing.T) {
	args := tunnelArgs("user@host", "/tmp/local.sock", "/home/user/.config/tsm/tsm.sock")
	assert.Equal(t, []string{
		"ssh", "-N", "-o", "ExitOnForwardFailure=yes",
		"-L", "/tmp/local.sock:/home/user/.config/tsm/tsm.sock",
		"user@host",
	}, args)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/remote/ -count=1 -v`
Expected: FAIL — package not found

**Step 3: Write minimal implementation**

Create `internal/remote/tunnel.go`:

```go
package remote

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// LocalSocketPath returns a deterministic temp socket path for a host.
func LocalSocketPath(host string) string {
	h := sha256.Sum256([]byte(host))
	return filepath.Join(os.TempDir(), fmt.Sprintf("tsm-%x.sock", h[:8]))
}

// tunnelArgs builds the ssh command arguments for the tunnel.
func tunnelArgs(host, localSock, remoteSock string) []string {
	fwd := fmt.Sprintf("%s:%s", localSock, remoteSock)
	return []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes", "-L", fwd, host}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/remote/ -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/remote/tunnel.go internal/remote/tunnel_test.go
git commit -m "feat(remote): tunnel socket path and SSH args"
```

---

### Task 3: Tunnel — Start, Close, resolveRemoteSocket

**Files:**
- Modify: `internal/remote/tunnel.go`
- Modify: `internal/remote/tunnel_test.go`

**Step 1: Write the failing tests**

Add to `internal/remote/tunnel_test.go`:

```go
func TestTunnel_Close_RemovesSocket(t *testing.T) {
	// Create a fake socket file
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	os.WriteFile(sockPath, []byte{}, 0o600)

	tun := &Tunnel{
		host:      "host",
		localSock: sockPath,
	}
	tun.Close()

	_, err := os.Stat(sockPath)
	assert.True(t, os.IsNotExist(err), "socket file should be removed")
}

func TestTunnel_ResolveRemoteSocket(t *testing.T) {
	// Mock cmdFactory that returns a fake remote path
	factory := func(name string, args ...string) CmdResult {
		return CmdResult{Output: "/home/wake/.config/tsm/tsm.sock\n", Err: nil}
	}
	tun := &Tunnel{host: "myhost", cmdRun: factory}
	path, err := tun.resolveRemoteSocket()
	assert.NoError(t, err)
	assert.Equal(t, "/home/wake/.config/tsm/tsm.sock", path)
}

func TestTunnel_ResolveRemoteSocket_Failure(t *testing.T) {
	factory := func(name string, args ...string) CmdResult {
		return CmdResult{Output: "", Err: fmt.Errorf("ssh: connect refused")}
	}
	tun := &Tunnel{host: "badhost", cmdRun: factory}
	_, err := tun.resolveRemoteSocket()
	assert.Error(t, err)
}

func TestTunnel_Start_FailsOnBadHost(t *testing.T) {
	factory := func(name string, args ...string) CmdResult {
		return CmdResult{Output: "", Err: fmt.Errorf("ssh: connect refused")}
	}
	tun := NewTunnel("badhost", WithCmdRun(factory))
	err := tun.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resolve remote socket")
}
```

Add `"fmt"`, `"os"`, `"path/filepath"` to imports.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/remote/ -run "TestTunnel_(Close|Resolve|Start)" -count=1 -v`
Expected: FAIL — types undefined

**Step 3: Write implementation**

Update `internal/remote/tunnel.go`:

```go
package remote

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CmdResult holds the result of a command execution.
type CmdResult struct {
	Output string
	Err    error
}

// CmdRunFunc executes a command and returns the result (injectable for testing).
type CmdRunFunc func(name string, args ...string) CmdResult

// defaultCmdRun uses os/exec.Command.Output().
func defaultCmdRun(name string, args ...string) CmdResult {
	out, err := exec.Command(name, args...).Output()
	return CmdResult{Output: string(out), Err: err}
}

// CmdStartFunc starts a command and returns process + wait func (injectable for testing).
type CmdStartFunc func(name string, args ...string) (Process, error)

// Process abstracts a running process for cleanup.
type Process interface {
	Kill() error
	Wait() error
}

func defaultCmdStart(name string, args ...string) (Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

// realProcess wraps *os.Process.
// (os.Process already satisfies the Process interface with Kill/Wait, but
// we need Wait to not error when called after Kill)

// Tunnel manages an SSH unix socket forwarding tunnel.
type Tunnel struct {
	host      string
	localSock string
	proc      Process
	cmdRun    CmdRunFunc
	cmdStart  CmdStartFunc
}

// TunnelOption configures a Tunnel.
type TunnelOption func(*Tunnel)

// WithCmdRun injects a CmdRunFunc (for testing).
func WithCmdRun(fn CmdRunFunc) TunnelOption {
	return func(t *Tunnel) { t.cmdRun = fn }
}

// WithCmdStart injects a CmdStartFunc (for testing).
func WithCmdStart(fn CmdStartFunc) TunnelOption {
	return func(t *Tunnel) { t.cmdStart = fn }
}

// NewTunnel creates a new Tunnel for the given SSH host.
func NewTunnel(host string, opts ...TunnelOption) *Tunnel {
	t := &Tunnel{
		host:      host,
		localSock: LocalSocketPath(host),
		cmdRun:    defaultCmdRun,
		cmdStart:  defaultCmdStart,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// LocalSocketPath returns a deterministic temp socket path for a host.
func LocalSocketPath(host string) string {
	h := sha256.Sum256([]byte(host))
	return filepath.Join(os.TempDir(), fmt.Sprintf("tsm-%x.sock", h[:8]))
}

// tunnelArgs builds the ssh command arguments for the tunnel.
func tunnelArgs(host, localSock, remoteSock string) []string {
	fwd := fmt.Sprintf("%s:%s", localSock, remoteSock)
	return []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes", "-L", fwd, host}
}

// Host returns the SSH host.
func (t *Tunnel) Host() string { return t.host }

// LocalSocket returns the local socket path.
func (t *Tunnel) LocalSocket() string { return t.localSock }

// Start establishes the SSH tunnel.
func (t *Tunnel) Start() error {
	remoteSock, err := t.resolveRemoteSocket()
	if err != nil {
		return fmt.Errorf("resolve remote socket: %w", err)
	}

	os.Remove(t.localSock)

	args := tunnelArgs(t.host, t.localSock, remoteSock)
	proc, err := t.cmdStart(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("start ssh tunnel: %w", err)
	}
	t.proc = proc

	if err := waitForSocket(t.localSock, 5*time.Second); err != nil {
		t.Close()
		return err
	}
	return nil
}

// Close shuts down the tunnel and cleans up.
func (t *Tunnel) Close() {
	if t.proc != nil {
		t.proc.Kill()
		t.proc.Wait()
		t.proc = nil
	}
	os.Remove(t.localSock)
}

func (t *Tunnel) resolveRemoteSocket() (string, error) {
	result := t.cmdRun("ssh", t.host, "echo", "$HOME/.config/tsm/tsm.sock")
	if result.Err != nil {
		return "", fmt.Errorf("ssh to %s: %w", t.host, result.Err)
	}
	path := strings.TrimSpace(result.Output)
	if path == "" {
		return "", fmt.Errorf("empty remote socket path from %s", t.host)
	}
	return path, nil
}

func waitForSocket(sockPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("tunnel socket %s not ready within %s", sockPath, timeout)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/remote/ -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/remote/tunnel.go internal/remote/tunnel_test.go
git commit -m "feat(remote): tunnel Start/Close with command injection"
```

---

### Task 4: Attach — SSH attach + exit code classification

**Files:**
- Create: `internal/remote/attach.go`
- Create: `internal/remote/attach_test.go`

**Step 1: Write the failing tests**

Create `internal/remote/attach_test.go`:

```go
package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyExit_Zero_IsDetach(t *testing.T) {
	assert.Equal(t, AttachDetached, classifyExit(0))
}

func TestClassifyExit_NonZero_IsDisconnect(t *testing.T) {
	assert.Equal(t, AttachDisconnected, classifyExit(1))
	assert.Equal(t, AttachDisconnected, classifyExit(255))
	assert.Equal(t, AttachDisconnected, classifyExit(-1))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/remote/ -run TestClassifyExit -count=1 -v`
Expected: FAIL — types undefined

**Step 3: Write implementation**

Create `internal/remote/attach.go`:

```go
package remote

import (
	"os"
	"os/exec"
)

// AttachResult represents the outcome of an SSH attach session.
type AttachResult int

const (
	AttachDetached     AttachResult = iota // exit 0: user detached normally
	AttachDisconnected                     // exit != 0: connection lost
)

// classifyExit maps an exit code to an AttachResult.
func classifyExit(exitCode int) AttachResult {
	if exitCode == 0 {
		return AttachDetached
	}
	return AttachDisconnected
}

// Attach spawns `ssh -t host tmux attach -t sessionName` and blocks until it exits.
// Returns AttachDetached if user detached, AttachDisconnected if connection was lost.
func Attach(host, sessionName string) AttachResult {
	cmd := exec.Command("ssh", "-t", host, "tmux", "attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return classifyExit(0)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return classifyExit(exitErr.ExitCode())
	}
	return AttachDisconnected
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/remote/ -run TestClassifyExit -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/remote/attach.go internal/remote/attach_test.go
git commit -m "feat(remote): attach with exit code classification"
```

---

### Task 5: Reconnect — backoff calculation + state types

**Files:**
- Create: `internal/remote/reconnect.go`
- Create: `internal/remote/reconnect_test.go`

**Step 1: Write the failing tests**

Create `internal/remote/reconnect_test.go`:

```go
package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff_FirstAttempt(t *testing.T) {
	d := backoff(0, 30*time.Second)
	assert.Equal(t, 1*time.Second, d)
}

func TestBackoff_ExponentialGrowth(t *testing.T) {
	assert.Equal(t, 1*time.Second, backoff(0, 30*time.Second))
	assert.Equal(t, 2*time.Second, backoff(1, 30*time.Second))
	assert.Equal(t, 4*time.Second, backoff(2, 30*time.Second))
	assert.Equal(t, 8*time.Second, backoff(3, 30*time.Second))
	assert.Equal(t, 16*time.Second, backoff(4, 30*time.Second))
}

func TestBackoff_CapsAtMax(t *testing.T) {
	assert.Equal(t, 30*time.Second, backoff(5, 30*time.Second))
	assert.Equal(t, 30*time.Second, backoff(10, 30*time.Second))
	assert.Equal(t, 30*time.Second, backoff(100, 30*time.Second))
}

func TestReconnState_String(t *testing.T) {
	assert.Equal(t, "disconnected", StateDisconnected.String())
	assert.Equal(t, "connecting", StateConnecting.String())
	assert.Equal(t, "connected", StateConnected.String())
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/remote/ -run "TestBackoff|TestReconnState" -count=1 -v`
Expected: FAIL — functions undefined

**Step 3: Write implementation**

Create `internal/remote/reconnect.go`:

```go
package remote

import "time"

// ReconnState represents the reconnection state.
type ReconnState int

const (
	StateDisconnected ReconnState = iota
	StateConnecting
	StateConnected
)

// String returns the state name.
func (s ReconnState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	default:
		return "unknown"
	}
}

// MaxBackoff is the maximum reconnection delay.
const MaxBackoff = 30 * time.Second

// backoff returns the delay for the given attempt number (0-indexed).
// Uses exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at max.
func backoff(attempt int, max time.Duration) time.Duration {
	d := time.Second << uint(attempt)
	if d > max {
		return max
	}
	return d
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/remote/ -run "TestBackoff|TestReconnState" -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/remote/reconnect.go internal/remote/reconnect_test.go
git commit -m "feat(remote): reconnect backoff and state types"
```

---

### Task 6: Reconnect Modal — Bubble Tea model

**Files:**
- Create: `internal/remote/model.go`
- Create: `internal/remote/model_test.go`

**Step 1: Write the failing tests**

Create `internal/remote/model_test.go`:

```go
package remote

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func applyKeyToReconn(m ReconnectModel, key string) ReconnectModel {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(ReconnectModel)
}

func TestReconnectModel_InitialState(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	assert.Equal(t, StateDisconnected, m.State())
	assert.Equal(t, 0, m.Attempt())
	assert.False(t, m.Quit())
	assert.False(t, m.BackToMenu())
}

func TestReconnectModel_EscGoesBackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd) // should be tea.Quit
}

func TestReconnectModel_QKeyQuits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	rm := applyKeyToReconn(m, "q")
	assert.True(t, rm.Quit())
}

func TestReconnectModel_View_ContainsHost(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	view := m.View()
	assert.Contains(t, view, "user@host")
}

func TestReconnectModel_StateTransition(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")

	// Simulate state change message
	updated, _ := m.Update(reconnStateMsg{state: StateConnecting, attempt: 1})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnecting, rm.State())
	assert.Equal(t, 1, rm.Attempt())
}

func TestReconnectModel_Connected_Quits(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(reconnStateMsg{state: StateConnected})
	rm := updated.(ReconnectModel)
	assert.Equal(t, StateConnected, rm.State())
	assert.NotNil(t, cmd) // should be tea.Quit to return control
}

func TestReconnectModel_SessionGone_BackToMenu(t *testing.T) {
	m := NewReconnectModel("user@host", "my-session")
	updated, cmd := m.Update(reconnSessionGoneMsg{})
	rm := updated.(ReconnectModel)
	assert.True(t, rm.BackToMenu())
	assert.NotNil(t, cmd)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/remote/ -run TestReconnectModel -count=1 -v`
Expected: FAIL — types undefined

**Step 3: Write implementation**

Create `internal/remote/model.go`:

```go
package remote

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// reconnStateMsg reports a state change from the reconnection goroutine.
type reconnStateMsg struct {
	state   ReconnState
	attempt int
}

// reconnSessionGoneMsg indicates the target session no longer exists.
type reconnSessionGoneMsg struct{}

// ReconnectModel is the Bubble Tea model for the reconnection modal.
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

// NewReconnectModel creates a new reconnect modal model.
func NewReconnectModel(host, session string) ReconnectModel {
	return ReconnectModel{
		host:    host,
		session: session,
		state:   StateDisconnected,
	}
}

func (m ReconnectModel) State() ReconnState { return m.state }
func (m ReconnectModel) Attempt() int       { return m.attempt }
func (m ReconnectModel) Quit() bool         { return m.quit }
func (m ReconnectModel) BackToMenu() bool   { return m.backToMenu }

func (m ReconnectModel) Init() tea.Cmd { return nil }

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
	case reconnStateMsg:
		m.state = msg.state
		m.attempt = msg.attempt
		if m.state == StateConnected {
			return m, tea.Quit
		}
		return m, nil
	case reconnSessionGoneMsg:
		m.backToMenu = true
		return m, tea.Quit
	case AnimTickMsg:
		m.frame++
		return m, animTickCmd()
	}
	return m, nil
}

// AnimTickMsg for reconnect modal animation (reuse from ui package concept).
type AnimTickMsg struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(200*__import_time.Millisecond, func(__import_time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

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

	// Center vertically and horizontally
	if m.width > 0 && m.height > 0 {
		style := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center)
		return style.Render(box)
	}
	return box
}
```

**Note:** Fix the `animTickCmd` import — use proper import for `time`:

```go
import "time"

func animTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/remote/ -run TestReconnectModel -count=1 -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/remote/model.go internal/remote/model_test.go
git commit -m "feat(remote): reconnect modal Bubble Tea model"
```

---

### Task 7: --remote flag + runRemote main loop

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: Write the failing test**

In `cmd/tsm/main_test.go`, add:

```go
func TestParseRemoteHost(t *testing.T) {
	host := parseRemoteHost([]string{"--remote", "myhost"})
	assert.Equal(t, "myhost", host)
}

func TestParseRemoteHost_Empty(t *testing.T) {
	host := parseRemoteHost([]string{"--inline"})
	assert.Equal(t, "", host)
}

func TestParseRemoteHost_NoValue(t *testing.T) {
	host := parseRemoteHost([]string{"--remote"})
	assert.Equal(t, "", host)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/tsm/ -run TestParseRemoteHost -count=1 -v`
Expected: FAIL — `parseRemoteHost` undefined

**Step 3: Write implementation**

In `cmd/tsm/main.go`, add the `parseRemoteHost` function and `runRemote` entry:

```go
// parseRemoteHost extracts the host from --remote <host> args.
func parseRemoteHost(args []string) string {
	for i, a := range args {
		if a == "--remote" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
```

In `main()`, add before the `--inline`/`--popup` check:

```go
if remoteHost := parseRemoteHost(args); remoteHost != "" {
	runRemote(remoteHost)
	return
}
```

Add `runRemote`:

```go
func runRemote(host string) {
	remote_ := remote.NewTunnel(host)
	if err := remote_.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法建立 SSH tunnel 到 %s: %v\n", host, err)
		fmt.Fprintf(os.Stderr, "請確認遠端主機的 tsm-daemon 已啟動 (tsm daemon start)\n")
		os.Exit(1)
	}
	defer remote_.Close()

	c, err := client.DialSocket(remote_.LocalSocket())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 遠端 %s 的 tsm-daemon 未回應: %v\n", host, err)
		fmt.Fprintf(os.Stderr, "請在遠端執行 tsm daemon start\n")
		os.Exit(1)
	}
	defer c.Close()

	for {
		// Show session picker
		deps := ui.Deps{Client: c, Cfg: loadConfig()}
		m := ui.NewModel(deps)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fm, ok := finalModel.(ui.Model)
		if !ok {
			return
		}

		selected := fm.Selected()
		if selected == "" {
			return // quit/esc
		}

		// Attach to remote session
		result := remote.Attach(host, selected)
		if result == remote.AttachDetached {
			continue // back to menu
		}

		// Disconnected — show reconnect modal
		rm := remote.NewReconnectModel(host, selected)
		reconnProg := tea.NewProgram(rm, tea.WithAltScreen())

		// Start reconnection in background
		go func() {
			attempt := 0
			for {
				attempt++
				reconnProg.Send(remote.ReconnStateMsg{State: remote.StateConnecting, Attempt: attempt})

				// Try to re-establish tunnel
				remote_.Close()
				if err := remote_.Start(); err != nil {
					wait := remote.Backoff(attempt-1, remote.MaxBackoff)
					time.Sleep(wait)
					continue
				}

				// Reconnect gRPC
				newC, err := client.DialSocket(remote_.LocalSocket())
				if err != nil {
					wait := remote.Backoff(attempt-1, remote.MaxBackoff)
					time.Sleep(wait)
					continue
				}

				// Check if session still exists
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				resp, err := newC.DaemonStatus(ctx)
				_ = resp
				cancel()
				if err != nil {
					newC.Close()
					wait := remote.Backoff(attempt-1, remote.MaxBackoff)
					time.Sleep(wait)
					continue
				}

				// Update the client reference
				c.Close()
				c = newC

				// Check session existence via Watch snapshot
				ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
				if err := c.Watch(ctx2); err == nil {
					if snap, err := c.RecvSnapshot(); err == nil {
						found := false
						for _, s := range snap.Sessions {
							if s.Name == selected {
								found = true
								break
							}
						}
						if !found {
							cancel2()
							reconnProg.Send(remote.ReconnSessionGoneMsg{})
							return
						}
					}
				}
				cancel2()

				reconnProg.Send(remote.ReconnStateMsg{State: remote.StateConnected})
				return
			}
		}()

		reconnFinal, _ := reconnProg.Run()
		rfm := reconnFinal.(remote.ReconnectModel)

		if rfm.Quit() {
			return
		}
		if rfm.BackToMenu() {
			continue
		}
		if rfm.State() == remote.StateConnected {
			// Reattach
			result := remote.Attach(host, selected)
			if result == remote.AttachDetached {
				continue
			}
			// Disconnected again — loop back to reconnect
			continue
		}
	}
}
```

Add imports: `"context"`, `"time"`, `"github.com/wake/tmux-session-menu/internal/remote"`.

**Note:** Export `ReconnStateMsg`, `ReconnSessionGoneMsg`, and `Backoff` from the remote package (rename from lowercase). Also export the `reconnStateMsg` fields.

**Step 4: Run tests and full build**

Run: `go test ./cmd/tsm/ -run TestParseRemoteHost -count=1 -v`
Expected: PASS

Run: `go build ./cmd/tsm/`
Expected: builds successfully

**Step 5: Commit**

```bash
git add cmd/tsm/main.go cmd/tsm/main_test.go
git commit -m "feat: add --remote flag with SSH tunnel and auto-reconnect"
```

---

### Task 8: Exports cleanup + full test suite + version bump

**Files:**
- Modify: `internal/remote/reconnect.go` — export `Backoff`
- Modify: `internal/remote/model.go` — export message types
- Modify: `VERSION`

**Step 1: Export necessary symbols**

In `internal/remote/reconnect.go`, rename `backoff` → `Backoff`.
Update `internal/remote/reconnect_test.go` references accordingly.

In `internal/remote/model.go`:
- Rename `reconnStateMsg` → `ReconnStateMsg` with exported fields `State`, `Attempt`
- Rename `reconnSessionGoneMsg` → `ReconnSessionGoneMsg`

**Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 3: Version bump**

Update `VERSION` from `0.13.0` to `0.14.0`.

**Step 4: Build**

Run: `make build`
Expected: builds successfully

**Step 5: Commit**

```bash
git add VERSION internal/remote/ cmd/tsm/
git commit -m "feat: tsm --remote v0.14.0 — SSH tunnel remote session management"
```

---

### Task 9: Update help text

**Files:**
- Modify: `cmd/tsm/main.go` — `printUsage()`

**Step 1: Update usage text**

Add to the Flags section in `printUsage()`:

```go
fmt.Fprintln(os.Stderr, "  --remote <host>    透過 SSH 連線遠端主機的 tsm-daemon")
```

**Step 2: Verify**

Run: `go run ./cmd/tsm/ --help`
Expected: shows `--remote` in flags

**Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "docs: add --remote to help text"
```
