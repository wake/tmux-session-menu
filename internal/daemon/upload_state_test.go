package daemon

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestUploadState_SetMode(t *testing.T) {
	us := NewUploadState()

	if us.IsUploadMode() {
		t.Fatal("expected upload mode off by default")
	}

	us.SetMode(true, "my-session")
	if !us.IsUploadMode() {
		t.Fatal("expected upload mode on")
	}
	if us.SessionName() != "my-session" {
		t.Errorf("got %q, want my-session", us.SessionName())
	}

	us.SetMode(false, "")
	if us.IsUploadMode() {
		t.Fatal("expected upload mode off")
	}
}

func TestUploadState_AddEvent(t *testing.T) {
	us := NewUploadState()

	us.AddEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 1024},
		},
	})

	events := us.DrainEvents()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Files[0].LocalPath != "/tmp/a.png" {
		t.Errorf("got %q", events[0].Files[0].LocalPath)
	}

	if len(us.DrainEvents()) != 0 {
		t.Fatal("expected empty after drain")
	}
}

func TestUploadState_SetModeOff_ClearsEvents(t *testing.T) {
	us := NewUploadState()
	us.SetMode(true, "sess")
	us.AddEvent(&tsmv1.UploadEvent{Files: []*tsmv1.UploadedFile{{LocalPath: "/tmp/x"}}})

	us.SetMode(false, "")

	if len(us.DrainEvents()) != 0 {
		t.Fatal("expected events cleared when mode turned off")
	}
}

func TestStateManager_UploadEventsInSnapshot(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)

	// 啟用上傳模式，BuildSnapshot 才會 drain 事件
	sm.uploadState.SetMode(true, "test-sess")

	sm.uploadState.AddEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/test.png", RemotePath: "/data/test.png"},
		},
	})

	snap := sm.BuildSnapshot()
	if len(snap.UploadEvents) != 1 {
		t.Fatalf("got %d upload events, want 1", len(snap.UploadEvents))
	}

	// drain 後第二次 snapshot 應為空
	snap2 := sm.BuildSnapshot()
	if len(snap2.UploadEvents) != 0 {
		t.Fatalf("got %d upload events after drain, want 0", len(snap2.UploadEvents))
	}
}
