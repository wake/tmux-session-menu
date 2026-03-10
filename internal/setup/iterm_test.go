package setup

import (
	"testing"
)

func TestItermCoprocessCommand(t *testing.T) {
	cmd := ItermCoprocessCommand("/usr/local/bin/tsm")
	want := `/usr/local/bin/tsm iterm-coprocess \(filenames)`
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}

func TestItermCoprocessCommand_WithSpaces(t *testing.T) {
	cmd := ItermCoprocessCommand("/path/to my/tsm")
	want := `/path/to my/tsm iterm-coprocess \(filenames)`
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}
