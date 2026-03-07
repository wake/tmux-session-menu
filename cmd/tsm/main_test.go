package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRemoteHost(t *testing.T) {
	host := parseRemoteHost([]string{"--remote", "myhost"})
	assert.Equal(t, "myhost", host)
}

func TestParseRemoteHost_Empty(t *testing.T) {
	host := parseRemoteHost([]string{"--inline"})
	assert.Equal(t, "", host)
}

func TestParseRemoteHost_NoValue(t *testing.T) {
	host := parseRemoteHost([]string{"--remote"})
	assert.Equal(t, "", host)
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
