package bind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall_NewFile(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Install(false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}

	data, _ := os.ReadFile(confPath)
	content := string(data)
	if !strings.Contains(content, markerBegin) {
		t.Error("missing marker begin")
	}
	if !strings.Contains(content, `bind-key -n C-q`) {
		t.Error("missing bind-key line")
	}
}

func TestInstall_ExistingContent(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	os.WriteFile(confPath, []byte("set -g mouse on\n"), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Install(false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}

	data, _ := os.ReadFile(confPath)
	content := string(data)
	if !strings.Contains(content, "set -g mouse on") {
		t.Error("original content lost")
	}
	if !strings.Contains(content, markerBegin) {
		t.Error("missing marker begin")
	}
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	os.WriteFile(confPath, []byte(bindBlock+"\n"), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Install(false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.Changed {
		t.Fatal("expected Changed=false for already installed")
	}
}

func TestInstall_DryRun(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Install(true)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}

	// dry-run 不應該寫入檔案
	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create file")
	}
}

func TestUninstall(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	content := "set -g mouse on\n" + bindBlock + "\nset -g status on\n"
	os.WriteFile(confPath, []byte(content), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Uninstall(false)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}

	data, _ := os.ReadFile(confPath)
	after := string(data)
	if strings.Contains(after, markerBegin) {
		t.Error("marker should be removed")
	}
	if !strings.Contains(after, "set -g mouse on") {
		t.Error("original content before block lost")
	}
	if !strings.Contains(after, "set -g status on") {
		t.Error("original content after block lost")
	}
}

func TestUninstall_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	os.WriteFile(confPath, []byte("set -g mouse on\n"), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Uninstall(false)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if result.Changed {
		t.Fatal("expected Changed=false")
	}
}

func TestRemoveBlock(t *testing.T) {
	input := "line1\n# [tsm] begin\nbind-key stuff\n# [tsm] end\nline2\n"
	got := removeBlock(input)
	if strings.Contains(got, "tsm") {
		t.Errorf("block not removed: %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("surrounding lines lost: %q", got)
	}
}
