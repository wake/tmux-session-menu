package remote

import (
	"fmt"
	"os"
	"path/filepath"
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
		"ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
		"-L", "/tmp/local.sock:/home/user/.config/tsm/tsm.sock",
		"user@host",
	}, args)
}

func TestTunnelArgs_IncludesKeepalive(t *testing.T) {
	args := tunnelArgs("user@host", "/tmp/l.sock", "/home/u/r.sock")
	assert.Contains(t, args, "ServerAliveInterval=5")
	assert.Contains(t, args, "ServerAliveCountMax=3")
}

func TestTunnel_Close_RemovesSocket(t *testing.T) {
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

func TestTunnelArgs_WithReverse(t *testing.T) {
	args := tunnelArgs("mlab", "/tmp/tsm-fwd.sock", "/home/user/.config/tsm/tsm.sock")
	assert.NotContains(t, args, "-R")

	args = tunnelArgsWithReverse(
		"mlab",
		"/tmp/tsm-fwd.sock",
		"/home/user/.config/tsm/tsm.sock",
		"/tmp/tsm-reverse-abc.sock",
		"/home/air/.config/tsm/tsm.sock",
	)
	assert.Contains(t, args, "-L")
	assert.Contains(t, args, "-R")

	for i, a := range args {
		if a == "-R" {
			assert.Equal(t, "/tmp/tsm-reverse-abc.sock:/home/air/.config/tsm/tsm.sock", args[i+1])
			break
		}
	}
}

func TestReverseSocketPath(t *testing.T) {
	path := ReverseSocketPath("mlab")
	assert.Contains(t, path, "tsm-hub-")
	assert.Contains(t, path, ".config/tsm/tsm-hub-")
	assert.Equal(t, path, ReverseSocketPath("mlab"))
	assert.NotEqual(t, path, ReverseSocketPath("air"))
}

func TestNewTunnel_WithReverse(t *testing.T) {
	tun := NewTunnel("mlab", WithReverse("/home/air/.config/tsm/tsm.sock"))
	assert.True(t, tun.hasReverse)
	assert.Equal(t, "/home/air/.config/tsm/tsm.sock", tun.reverseLocalSock)
	assert.Equal(t, ReverseSocketPath("mlab"), tun.reverseRemoteSock)
}

func TestTunnel_Start_WithReverse_CleansStaleSocket(t *testing.T) {
	var cmds []string
	factory := func(name string, args ...string) CmdResult {
		cmd := name + " " + strings.Join(args, " ")
		cmds = append(cmds, cmd)
		if strings.Contains(cmd, "echo") {
			return CmdResult{Output: "/home/user/.config/tsm/tsm.sock"}
		}
		return CmdResult{}
	}
	startFn := func(name string, args ...string) (Process, error) {
		return nil, fmt.Errorf("intentional stop")
	}
	tun := NewTunnel("mlab",
		WithCmdRun(factory),
		WithCmdStart(startFn),
		WithReverse("/home/air/.config/tsm/tsm.sock"),
	)
	_ = tun.Start() // 會因 startFn 而失敗，但 rm -f 應已執行

	// 確認在 SSH tunnel 啟動前先清除遠端 stale socket
	var foundRm bool
	for _, c := range cmds {
		if strings.Contains(c, "rm -f") && strings.Contains(c, "tsm-hub-") {
			foundRm = true
		}
	}
	assert.True(t, foundRm, "should ssh rm -f stale reverse socket before tunnel start")
}
