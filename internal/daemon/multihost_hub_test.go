package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

func TestMultiHostHub_RegisterAndBroadcast(t *testing.T) {
	hub := NewMultiHostHub()
	defer hub.Close()

	ch := hub.Register()
	snap := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{HostId: "local", Name: "air", Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED},
		},
	}

	hub.Broadcast(snap)

	select {
	case got := <-ch:
		assert.Equal(t, "local", got.Hosts[0].HostId)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestMultiHostHub_Unregister(t *testing.T) {
	hub := NewMultiHostHub()
	defer hub.Close()

	ch := hub.Register()
	hub.Unregister(ch)

	_, ok := <-ch
	assert.False(t, ok)
}

func TestMultiHostHub_Count(t *testing.T) {
	hub := NewMultiHostHub()
	defer hub.Close()

	assert.Equal(t, 0, hub.Count())
	ch := hub.Register()
	assert.Equal(t, 1, hub.Count())
	hub.Unregister(ch)
	assert.Equal(t, 0, hub.Count())
}
