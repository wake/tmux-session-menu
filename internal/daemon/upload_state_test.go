package daemon

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
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
