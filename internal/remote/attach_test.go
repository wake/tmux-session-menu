package remote

import (
	"strings"
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

func TestSetHubSocket_CallsSSHWithCorrectArgs(t *testing.T) {
	var capturedName string
	var capturedArgs []string
	orig := sshRunFn
	sshRunFn = func(name string, args ...string) error {
		capturedName = name
		capturedArgs = args
		return nil
	}
	defer func() { sshRunFn = orig }()

	err := SetHubSocket("user@host", "/tmp/tsm-hub.sock")
	if err != nil {
		t.Fatalf("SetHubSocket unexpected error: %v", err)
	}
	assert.Equal(t, "ssh", capturedName)
	assert.Contains(t, capturedArgs, "user@host")
	// 使用 login shell 確保 tmux 在 PATH 中
	assert.Contains(t, capturedArgs, "bash")
	assert.Contains(t, capturedArgs, "-lc")
	argsStr := strings.Join(capturedArgs, " ")
	assert.Contains(t, argsStr, "@tsm_hub_socket")
	assert.Contains(t, argsStr, "/tmp/tsm-hub.sock")
	assert.Contains(t, argsStr, "set-option")
}

func TestClearHubSocket_CallsSSHWithUnsetFlag(t *testing.T) {
	var capturedArgs []string
	orig := sshRunFn
	sshRunFn = func(name string, args ...string) error {
		capturedArgs = args
		return nil
	}
	defer func() { sshRunFn = orig }()

	err := ClearHubSocket("user@host")
	if err != nil {
		t.Fatalf("ClearHubSocket unexpected error: %v", err)
	}
	assert.Contains(t, capturedArgs, "user@host")
	// 使用 login shell 確保 tmux 在 PATH 中
	assert.Contains(t, capturedArgs, "bash")
	assert.Contains(t, capturedArgs, "-lc")
	argsStr := strings.Join(capturedArgs, " ")
	assert.Contains(t, argsStr, "@tsm_hub_socket")
	assert.Contains(t, argsStr, "set-option")
	assert.Contains(t, argsStr, "-gu")
}
