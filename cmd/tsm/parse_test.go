package main

import (
	"testing"
)

func TestParseRemoteHosts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "無 remote 參數",
			args: []string{"--inline"},
			want: nil,
		},
		{
			name: "單一 remote",
			args: []string{"--remote", "hostA"},
			want: []string{"hostA"},
		},
		{
			name: "多個 remote",
			args: []string{"--remote", "hostA", "--remote", "hostB"},
			want: []string{"hostA", "hostB"},
		},
		{
			name: "三個 remote",
			args: []string{"--remote", "hostA", "--remote", "hostB", "--remote", "hostC"},
			want: []string{"hostA", "hostB", "hostC"},
		},
		{
			name: "remote 混合其他 flag",
			args: []string{"--inline", "--remote", "hostA", "--popup", "--remote", "hostB"},
			want: []string{"hostA", "hostB"},
		},
		{
			name: "尾端 remote 缺少值",
			args: []string{"--remote", "hostA", "--remote"},
			want: []string{"hostA"},
		},
		{
			name: "僅有尾端 remote 缺少值",
			args: []string{"--remote"},
			want: nil,
		},
		{
			name: "空 args",
			args: []string{},
			want: nil,
		},
		{
			name: "remote 值不應包含其他 flag",
			args: []string{"--remote", "--inline"},
			want: []string{"--inline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRemoteHosts(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("parseRemoteHosts(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseRemoteHosts(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}
