package upgrade

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPostUpgrade_HappyPath(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			calls = append(calls, "install:"+tmpPath)
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove:"+path)
		},
		StopDaemon: func() error {
			calls = append(calls, "stop")
			return nil
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec:"+path)
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", []string{"tsm", "--inline"}, []string{"HOME=/root"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"install:/tmp/tsm-new",
		"remove:/tmp/tsm-new",
		"stop",
		"start",
		"exec:/usr/local/bin/tsm",
	}, calls)
}

func TestRunPostUpgrade_InstallFail(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			return "", fmt.Errorf("permission denied")
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove")
		},
		StopDaemon: func() error {
			calls = append(calls, "stop")
			return nil
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec")
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "安裝失敗")
	// Install fails → remove tmp, but NO stop/start/exec
	assert.Equal(t, []string{"remove"}, calls)
}

func TestRunPostUpgrade_DaemonStopFail_ContinuesAnyway(t *testing.T) {
	var calls []string

	ops := PostUpgradeOps{
		InstallBinary: func(tmpPath string) (string, error) {
			calls = append(calls, "install")
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove")
		},
		StopDaemon: func() error {
			calls = append(calls, "stop-fail")
			return fmt.Errorf("no daemon running")
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
		Exec: func(path string, args []string, env []string) error {
			calls = append(calls, "exec")
			return nil
		},
	}

	err := RunPostUpgrade(ops, "/tmp/tsm-new", nil, nil)
	require.NoError(t, err)
	// stop fails but flow continues
	assert.Equal(t, []string{"install", "remove", "stop-fail", "start", "exec"}, calls)
}
