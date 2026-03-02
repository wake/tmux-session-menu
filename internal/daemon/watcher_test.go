package daemon

import (
	"testing"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatcherHub_RegisterAndBroadcast(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	ch := hub.Register()
	defer hub.Unregister(ch)

	snap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "test"}},
	}
	hub.Broadcast(snap)

	select {
	case got := <-ch:
		require.Len(t, got.Sessions, 1)
		assert.Equal(t, "test", got.Sessions[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestWatcherHub_MultipleWatchers(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	ch1 := hub.Register()
	defer hub.Unregister(ch1)
	ch2 := hub.Register()
	defer hub.Unregister(ch2)

	snap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "s1"}},
	}
	hub.Broadcast(snap)

	for _, ch := range []<-chan *tsmv1.StateSnapshot{ch1, ch2} {
		select {
		case got := <-ch:
			assert.Equal(t, "s1", got.Sessions[0].Name)
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestWatcherHub_Unregister(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	ch := hub.Register()
	hub.Unregister(ch)

	assert.Equal(t, 0, hub.Count())
}

func TestWatcherHub_SlowConsumerNonBlocking(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	ch := hub.Register()
	defer hub.Unregister(ch)

	// 廣播兩次但不讀取 — buffer=1 所以第二次應非阻塞地覆蓋
	snap1 := &tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "old"}}}
	snap2 := &tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "new"}}}

	hub.Broadcast(snap1)
	hub.Broadcast(snap2)

	// 應該能讀到最新的快照
	select {
	case got := <-ch:
		assert.Equal(t, "new", got.Sessions[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestWatcherHub_Count(t *testing.T) {
	hub := NewWatcherHub()
	defer hub.Close()

	assert.Equal(t, 0, hub.Count())

	ch1 := hub.Register()
	assert.Equal(t, 1, hub.Count())

	ch2 := hub.Register()
	assert.Equal(t, 2, hub.Count())

	hub.Unregister(ch1)
	assert.Equal(t, 1, hub.Count())

	hub.Unregister(ch2)
	assert.Equal(t, 0, hub.Count())
}

func TestWatcherHub_BroadcastAfterClose(t *testing.T) {
	hub := NewWatcherHub()
	ch := hub.Register()
	hub.Close()

	// Close 後廣播不應 panic
	hub.Broadcast(&tsmv1.StateSnapshot{})
	_ = ch
}
