package ui

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

func TestUploadModal_InitialState(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects/myapp")
	if m.sessionName != "my-session" {
		t.Errorf("got session %q", m.sessionName)
	}
	if len(m.results) != 0 {
		t.Error("expected empty results")
	}
}

func TestUploadModal_AddEvent_Success(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects/myapp")

	m.addEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 1024},
		},
	})

	if len(m.results) != 1 {
		t.Fatalf("got %d results, want 1", len(m.results))
	}
	if m.results[0].err != "" {
		t.Errorf("expected no error, got %q", m.results[0].err)
	}
}

func TestUploadModal_AddEvent_Error(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects/myapp")

	m.addEvent(&tsmv1.UploadEvent{
		Error: "scp timeout",
	})

	if len(m.results) != 1 {
		t.Fatalf("got %d results, want 1", len(m.results))
	}
	if m.results[0].err != "scp timeout" {
		t.Errorf("got err %q", m.results[0].err)
	}
}

func TestUploadModal_MultipleEvents(t *testing.T) {
	m := newUploadModal("s", "", "/tmp")
	m.addEvent(&tsmv1.UploadEvent{Files: []*tsmv1.UploadedFile{{LocalPath: "/a"}}})
	m.addEvent(&tsmv1.UploadEvent{Files: []*tsmv1.UploadedFile{{LocalPath: "/b"}}})
	m.addEvent(&tsmv1.UploadEvent{Error: "fail"})

	if len(m.results) != 3 {
		t.Fatalf("got %d results, want 3", len(m.results))
	}
}

func TestUploadModal_View_Empty(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects")
	v := m.view()
	if v == "" {
		t.Error("view should not be empty")
	}
}

func TestFormatUploadSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{500, "500 B"},
		{1536, "1.5 KB"},
		{1572864, "1.5 MB"},
		{1610612736, "1.5 GB"},
	}
	for _, tt := range tests {
		got := formatUploadSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatUploadSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
