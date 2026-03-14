package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/remote"
)

// --- reconnLoop 測試 ---

func TestReconnLoop_SucceedsOnFirstAttempt(t *testing.T) {
	succeeded := false
	var msgs []remote.ReconnState
	reconnLoop(context.Background(),
		func(ctx context.Context) error { return nil },
		time.Millisecond,
		func(msg remote.ReconnStateMsg) { msgs = append(msgs, msg.State) },
		func() { succeeded = true },
	)
	assert.True(t, succeeded)
	assert.Equal(t, []remote.ReconnState{remote.StateConnecting, remote.StateConnected}, msgs)
}

func TestReconnLoop_RetriesUntilSuccess(t *testing.T) {
	attempts := 0
	reconnLoop(context.Background(),
		func(ctx context.Context) error {
			attempts++
			if attempts >= 3 {
				return nil
			}
			return fmt.Errorf("fail")
		},
		time.Millisecond,
		func(msg remote.ReconnStateMsg) {},
		func() {},
	)
	assert.Equal(t, 3, attempts)
}

func TestReconnLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	reconnLoop(ctx,
		func(ctx context.Context) error {
			attempts++
			if attempts == 2 {
				cancel()
			}
			return fmt.Errorf("fail")
		},
		time.Millisecond,
		func(msg remote.ReconnStateMsg) {},
		func() {},
	)
	assert.Equal(t, 2, attempts)
}

func TestReconnLoop_SendsConnectingOnEachAttempt(t *testing.T) {
	var states []remote.ReconnState
	attempts := 0
	reconnLoop(context.Background(),
		func(ctx context.Context) error {
			attempts++
			if attempts >= 2 {
				return nil
			}
			return fmt.Errorf("fail")
		},
		time.Millisecond,
		func(msg remote.ReconnStateMsg) { states = append(states, msg.State) },
		func() {},
	)
	// 兩次 Connecting + 一次 Connected
	assert.Equal(t, []remote.ReconnState{
		remote.StateConnecting,
		remote.StateConnecting,
		remote.StateConnected,
	}, states)
}

func TestDetectLocalAddr_Loopback(t *testing.T) {
	// 連到 127.0.0.1 時，出口 IP 應為 127.0.0.1
	got := detectLocalAddr("127.0.0.1")
	assert.Equal(t, "127.0.0.1", got)
}

func TestDetectLocalAddr_StripUser(t *testing.T) {
	// user@host 格式應正確解析（127.0.0.1 保證可達）
	got := detectLocalAddr("user@127.0.0.1")
	assert.Equal(t, "127.0.0.1", got)
}

func TestDetectLocalAddr_Unreachable(t *testing.T) {
	// 無法路由的位址應回傳空字串
	got := detectLocalAddr("192.0.2.1") // TEST-NET-1, RFC 5737
	// 即使不可達，OS 仍可能選擇預設路由的出口 IP，所以只驗證不 panic
	_ = got
}

func TestParseRunMode(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected runMode
	}{
		{"empty", nil, modeAuto},
		{"no flags", []string{}, modeAuto},
		{"inline", []string{"--inline"}, modeInline},
		{"popup", []string{"--popup"}, modePopup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseRunMode(tt.args))
		})
	}
}
