package status

import (
	"testing"
	"time"
)

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{100, "100"},
		{1000, "1,000"},
		{1250000, "1,250,000"},
		{999999999, "999,999,999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatNumber(tt.input)
			if got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  string
	}{
		{"seconds", 6 * time.Second, "6s"},
		{"under 1 minute", 45 * time.Second, "45s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48 * time.Hour, "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.input)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRepeat(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"", 5, ""},
		{"a", 0, ""},
		{"a", 1, "a"},
		{"ab", 3, "ababab"},
		{"─", 5, "─────"},
		{"x", -1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := repeat(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("repeat(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}
