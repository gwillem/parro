package main

import (
	"testing"
	"time"
)

func TestParseDelta(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"2h", 2 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"1d", 24 * time.Hour},
		{"90m", 90 * time.Minute},
	}
	for _, tt := range tests {
		got, err := parseDelta(tt.input)
		if err != nil {
			t.Errorf("parseDelta(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDelta(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseDeltaErrors(t *testing.T) {
	bad := []string{"", "5", "d", "5x", "-3h", "abc", "3.5h"}
	for _, s := range bad {
		if _, err := parseDelta(s); err == nil {
			t.Errorf("parseDelta(%q) should fail", s)
		}
	}
}
