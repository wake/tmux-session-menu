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
