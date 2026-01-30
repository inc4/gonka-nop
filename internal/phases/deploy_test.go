package phases

import "testing"

func TestFormatBlockHeight(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{100, "100"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1200000, "1,200,000"},
		{1250000, "1,250,000"},
		{999999999, "999,999,999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatBlockHeight(tt.input)
			if got != tt.want {
				t.Errorf("formatBlockHeight(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
