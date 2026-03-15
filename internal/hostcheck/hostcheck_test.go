package hostcheck

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckLocal_TmuxFound(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) {
			if file == "tmux" {
				return "/usr/bin/tmux", nil
			}
			return "", &lookPathError{file}
		},
	}
	r := c.CheckLocal()
	assert.Equal(t, StatusReady, r.Status)
	assert.Nil(t, r.SSH, "local 不檢查 SSH")
	assert.NotNil(t, r.Tmux)
	assert.True(t, *r.Tmux)
}

func TestCheckLocal_TmuxNotFound(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) {
			return "", &lookPathError{file}
		},
	}
	r := c.CheckLocal()
	assert.Equal(t, StatusFailed, r.Status)
	assert.NotNil(t, r.Tmux)
	assert.False(t, *r.Tmux)
	assert.Contains(t, r.Message, "tmux")
}

func TestCheckRemote_AllPass(t *testing.T) {
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			return nil
		},
	}
	r := c.CheckRemote("mlab")
	assert.Equal(t, StatusReady, r.Status)
	assert.True(t, *r.SSH)
	assert.True(t, *r.Tmux)
}

func TestCheckRemote_SSHFail(t *testing.T) {
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			return fmt.Errorf("connection refused")
		},
	}
	r := c.CheckRemote("bad-host")
	assert.Equal(t, StatusFailed, r.Status)
	assert.False(t, *r.SSH)
	assert.Nil(t, r.Tmux, "SSH 失敗時不檢查 tmux")
	assert.Contains(t, r.Message, "SSH")
}

func TestCheckRemote_SSHOk_TmuxMissing(t *testing.T) {
	callCount := 0
	c := &Checker{
		ExecFn: func(name string, args ...string) error {
			callCount++
			if callCount == 1 {
				return nil
			}
			return fmt.Errorf("tmux not found")
		},
	}
	r := c.CheckRemote("mlab")
	assert.Equal(t, StatusFailed, r.Status)
	assert.True(t, *r.SSH)
	assert.False(t, *r.Tmux)
	assert.Contains(t, r.Message, "tmux")
}

func TestCheckAll_Parallel(t *testing.T) {
	c := &Checker{
		LookPathFn: func(file string) (string, error) { return "/usr/bin/tmux", nil },
		ExecFn:     func(name string, args ...string) error { return nil },
	}
	hosts := map[string]string{
		"local": "",
		"mlab1": "mlab1",
		"mlab2": "mlab2",
	}
	results := c.CheckAll(hosts, 5)
	assert.Len(t, results, 3)
	for name, r := range results {
		assert.Equal(t, StatusReady, r.Status, "host %s should be ready", name)
	}
}

func TestCheckAll_SemaphoreLimit(t *testing.T) {
	var mu sync.Mutex
	maxSeen := 0
	current := 0

	c := &Checker{
		LookPathFn: func(file string) (string, error) { return "/usr/bin/tmux", nil },
		ExecFn: func(name string, args ...string) error {
			mu.Lock()
			current++
			if current > maxSeen {
				maxSeen = current
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
			return nil
		},
	}
	hosts := map[string]string{}
	for i := 0; i < 10; i++ {
		hosts[fmt.Sprintf("host%d", i)] = fmt.Sprintf("host%d", i)
	}
	_ = c.CheckAll(hosts, 3)
	assert.LessOrEqual(t, maxSeen, 3, "concurrent checks should not exceed semaphore limit")
}
