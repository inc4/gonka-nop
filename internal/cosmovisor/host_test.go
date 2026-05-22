package cosmovisor

import "testing"

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "simple path",
			input:  "/home/user/gonka",
			expect: "'/home/user/gonka'",
		},
		{
			name:   "path with spaces",
			input:  "/home/user/my node",
			expect: "'/home/user/my node'",
		},
		{
			name:   "path with single quote",
			input:  "/home/user/it's",
			expect: "'/home/user/it'\\''s'",
		},
		{
			name:   "empty string",
			input:  "",
			expect: "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShellQuote(tt.input)
			if got != tt.expect {
				t.Errorf("ShellQuote(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
