package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRemoteHosts_Single(t *testing.T) {
	hosts := parseRemoteHosts([]string{"--remote", "myhost"})
	assert.Equal(t, []string{"myhost"}, hosts)
}

func TestParseRemoteHosts_Empty(t *testing.T) {
	hosts := parseRemoteHosts([]string{"--inline"})
	assert.Nil(t, hosts)
}

func TestParseRemoteHosts_NoValue(t *testing.T) {
	hosts := parseRemoteHosts([]string{"--remote"})
	assert.Nil(t, hosts)
}

func TestParseRunMode(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected runMode
	}{
		{"empty", nil, modeAuto},
		{"no flags", []string{}, modeAuto},
		{"inline", []string{"--inline"}, modeInline},
		{"popup", []string{"--popup"}, modePopup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseRunMode(tt.args))
		})
	}
}
