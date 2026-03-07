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
