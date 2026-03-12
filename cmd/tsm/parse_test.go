package main

import (
	"testing"
)

func TestParseHostFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "無 host 參數", args: []string{"--inline"}, want: nil},
		{name: "單一 host", args: []string{"--host", "hostA"}, want: []string{"hostA"}},
		{name: "多個 host", args: []string{"--host", "hostA", "--host", "hostB"}, want: []string{"hostA", "hostB"}},
		{name: "host 混合其他 flag", args: []string{"--inline", "--host", "hostA", "--local", "--host", "hostB"}, want: []string{"hostA", "hostB"}},
		{name: "尾端 host 缺少值", args: []string{"--host", "hostA", "--host"}, want: []string{"hostA"}},
		{name: "空 args", args: []string{}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHostFlags(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("parseHostFlags(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseHostFlags(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseLocalFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "無 local", args: []string{"--host", "a"}, want: false},
		{name: "有 local", args: []string{"--local"}, want: true},
		{name: "local 與 host 混合", args: []string{"--local", "--host", "a"}, want: true},
		{name: "空 args", args: []string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLocalFlag(tt.args)
			if got != tt.want {
				t.Errorf("parseLocalFlag(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildPopupHostArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "純 inline 無 host", args: []string{"--inline"}, want: ""},
		{name: "裸 --host", args: []string{"--host", "--inline"}, want: "--host"},
		{name: "--host 帶值", args: []string{"--host", "air-2019", "--inline"}, want: "--host air-2019"},
		{name: "--local", args: []string{"--local", "--inline"}, want: "--local"},
		{name: "--local 加 --host 帶值", args: []string{"--local", "--host", "air-2019", "--inline"}, want: "--local --host air-2019"},
		{name: "多個 --host", args: []string{"--inline", "--host", "a", "--host", "b"}, want: "--host a --host b"},
		{name: "空 args", args: []string{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPopupHostArgs(tt.args)
			if got != tt.want {
				t.Errorf("buildPopupHostArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestSyncPopupArgs(t *testing.T) {
	tests := []struct {
		name     string
		hostArgs string
		wantArgs []string
	}{
		{
			name:     "空字串設定空值",
			hostArgs: "",
			wantArgs: []string{"set-option", "-g", "@tsm_popup_args", ""},
		},
		{
			name:     "裸 --host",
			hostArgs: "--host",
			wantArgs: []string{"set-option", "-g", "@tsm_popup_args", "--host"},
		},
		{
			name:     "--host 帶值",
			hostArgs: "--host air-2019",
			wantArgs: []string{"set-option", "-g", "@tsm_popup_args", "--host air-2019"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured []string
			orig := tmuxExecFn
			tmuxExecFn = func(args ...string) { captured = args }
			defer func() { tmuxExecFn = orig }()

			syncPopupArgs(tt.hostArgs)

			if len(captured) != len(tt.wantArgs) {
				t.Fatalf("args 長度不符: got %v, want %v", captured, tt.wantArgs)
			}
			for i := range captured {
				if captured[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, captured[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestHasHostMode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "裸 --host", args: []string{"--host"}, want: true},
		{name: "有值 --host", args: []string{"--host", "a"}, want: true},
		{name: "無 --host", args: []string{"--inline"}, want: false},
		{name: "--local 也算 host 模式", args: []string{"--local"}, want: true},
		{name: "空 args", args: []string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasHostMode(tt.args)
			if got != tt.want {
				t.Errorf("hasHostMode(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestParseHubSocket(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "無 hub-socket", args: []string{"--inline"}, want: ""},
		{name: "有 hub-socket", args: []string{"--hub-socket", "/tmp/tsm-hub-abc.sock", "--inline"}, want: "/tmp/tsm-hub-abc.sock"},
		{name: "尾端缺值", args: []string{"--hub-socket"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHubSocket(tt.args)
			if got != tt.want {
				t.Errorf("parseHubSocket(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
