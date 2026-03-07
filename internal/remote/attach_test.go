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

func TestAttachArgs_IncludesKeepalive(t *testing.T) {
	args := attachArgs("user@host", "my-session")
	assert.Contains(t, args, "ServerAliveInterval=5")
	assert.Contains(t, args, "ServerAliveCountMax=3")
}

func TestAttachArgs_Format(t *testing.T) {
	args := attachArgs("user@host", "my-session")
	assert.Equal(t, "-t", args[0])
	assert.Contains(t, args, "user@host")
}
