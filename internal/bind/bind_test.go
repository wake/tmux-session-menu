package bind

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Detect ──────────────────────────────────────────────────

func TestDetect_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	// 空檔案
	os.WriteFile(confPath, []byte(""), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	r := Detect()
	if r.Status != BindNotInstalled {
		t.Fatalf("expected BindNotInstalled, got %d", r.Status)
	}
	if r.ConfPath != confPath {
		t.Fatalf("expected ConfPath=%s, got %s", confPath, r.ConfPath)
	}
}

func TestDetect_Installed(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	os.WriteFile(confPath, []byte(bindBlock+"\n"), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	r := Detect()
	if r.Status != BindInstalled {
		t.Fatalf("expected BindInstalled, got %d", r.Status)
	}
	if r.ConfPath != confPath {
		t.Fatalf("expected ConfPath=%s, got %s", confPath, r.ConfPath)
	}
}

func TestDetect_NoConfFile(t *testing.T) {
	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return "", errors.New("no home") }
	defer func() { tmuxConfPathFn = origFn }()

	r := Detect()
	if r.Status != BindNoConfFile {
		t.Fatalf("expected BindNoConfFile, got %d", r.Status)
	}
}

func TestDetect_FileNotExist(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf") // 不建立檔案

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	r := Detect()
	if r.Status != BindNotInstalled {
		t.Fatalf("expected BindNotInstalled for non-existent file, got %d", r.Status)
	}
}

// ─── BuildComponent ─────────────────────────────────────────

func TestBuildComponent_NotInstalled(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	comp := BuildComponent()
	if !comp.Checked {
		t.Error("expected Checked=true for NotInstalled")
	}
	if comp.Disabled {
		t.Error("expected Disabled=false for NotInstalled")
	}
	if !strings.Contains(comp.Note, "將寫入") {
		t.Errorf("expected Note to contain '將寫入', got %q", comp.Note)
	}
}

func TestBuildComponent_Installed(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	os.WriteFile(confPath, []byte(bindBlock+"\n"), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	comp := BuildComponent()
	if comp.Checked {
		t.Error("expected Checked=false for Installed")
	}
	if comp.Disabled {
		t.Error("expected Disabled=false for Installed")
	}
	if !strings.Contains(comp.Note, "已安裝於") {
		t.Errorf("expected Note to contain '已安裝於', got %q", comp.Note)
	}
}

func TestBuildComponent_NoConfFile(t *testing.T) {
	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return "", errors.New("no home") }
	defer func() { tmuxConfPathFn = origFn }()

	comp := BuildComponent()
	if comp.Checked {
		t.Error("expected Checked=false for NoConfFile")
	}
	if !comp.Disabled {
		t.Error("expected Disabled=true for NoConfFile")
	}
}

// ─── Install / Uninstall ────────────────────────────────────

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

func TestInstall_ContainsReadOnlyKeyTable(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	_, err := Install(false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, _ := os.ReadFile(confPath)
	content := string(data)

	// tsm-readonly key table 應包含 C-q 綁定（唯讀模式下唯一可用的主要按鍵）
	if !strings.Contains(content, "-T tsm-readonly") {
		t.Error("missing tsm-readonly key table binding")
	}
	if !strings.Contains(content, "tsm-readonly C-q") {
		t.Error("missing C-q binding in tsm-readonly table")
	}

	// tsm-readonly-prefix key table 應包含 d → detach（安全出口）
	if !strings.Contains(content, "-T tsm-readonly-prefix") {
		t.Error("missing tsm-readonly-prefix key table binding")
	}
	if !strings.Contains(content, "tsm-readonly-prefix d detach-client") {
		t.Error("missing detach-client binding in tsm-readonly-prefix table")
	}

}

func TestInstall_UpgradesOutdatedBlock(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, ".tmux.conf")
	// 舊版 bind block（不含 tsm-readonly）
	oldBlock := "set -g mouse on\n# [tsm] begin\nbind-key -n C-q display-popup -E -w 80% -h 80% \"tsm --inline\"\n# [tsm] end\n"
	os.WriteFile(confPath, []byte(oldBlock), 0o644)

	origFn := tmuxConfPathFn
	tmuxConfPathFn = func() (string, error) { return confPath, nil }
	defer func() { tmuxConfPathFn = origFn }()

	result, err := Install(false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected Changed=true for outdated block upgrade")
	}

	data, _ := os.ReadFile(confPath)
	content := string(data)

	// 原始內容應保留
	if !strings.Contains(content, "set -g mouse on") {
		t.Error("original content lost")
	}
	// 新 block 應包含 tsm-readonly
	if !strings.Contains(content, "tsm-readonly") {
		t.Error("upgraded block should contain tsm-readonly bindings")
	}
	// marker 應只出現一次
	if strings.Count(content, markerBegin) != 1 {
		t.Errorf("expected exactly 1 marker begin, got %d", strings.Count(content, markerBegin))
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
