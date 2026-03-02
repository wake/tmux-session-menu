package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
