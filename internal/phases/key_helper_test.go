package phases

import "testing"

func TestParseKeyOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantAddr string
		wantName string
	}{
		{
			name:     "Valid",
			input:    `{"name":"gonka-cold","type":"local","address":"gonka1abc123def456","pubkey":"gonkapub1addwnpepq..."}`,
			wantAddr: "gonka1abc123def456",
			wantName: "gonka-cold",
		},
		{
			name: "With whitespace",
			input: `  {"name":"gonka-warm","type":"local","address":"gonka1xyz789","pubkey":"gonkapub1addwnpepq..."}
`,
			wantAddr: "gonka1xyz789",
			wantName: "gonka-warm",
		},
		{
			name:    "Empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Invalid JSON",
			input:   "not json at all",
			wantErr: true,
		},
		{
			name:    "Missing address",
			input:   `{"name":"test","type":"local","address":"","pubkey":"abc"}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := ParseKeyOutput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if key.Address != tt.wantAddr {
				t.Errorf("Address = %q, want %q", key.Address, tt.wantAddr)
			}
			if key.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", key.Name, tt.wantName)
			}
		})
	}
}

func TestExtractMnemonic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Standard mnemonic",
			input: "some warning message\nanother line\nword1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12\n",
			want:  "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10 word11 word12",
		},
		{
			name:  "24 word mnemonic",
			input: "w1 w2 w3 w4 w5 w6 w7 w8 w9 w10 w11 w12 w13 w14 w15 w16 w17 w18 w19 w20 w21 w22 w23 w24",
			want:  "w1 w2 w3 w4 w5 w6 w7 w8 w9 w10 w11 w12 w13 w14 w15 w16 w17 w18 w19 w20 w21 w22 w23 w24",
		},
		{
			name:  "No mnemonic",
			input: "short line\nanother short",
			want:  "",
		},
		{
			name:  "Empty",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMnemonic(tt.input)
			if got != tt.want {
				t.Errorf("ExtractMnemonic() = %q, want %q", got, tt.want)
			}
		})
	}
}
