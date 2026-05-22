package cosmovisor

import "testing"

func TestValidateReleaseURL_Table(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Happy paths
		{
			name: "gonka-ai/gonka release",
			url:  "https://github.com/gonka-ai/gonka/releases/download/release%2Fv0.2.12/inferenced-amd64.zip",
		},
		{
			name: "product-science/race-releases api-post3",
			url:  "https://github.com/product-science/race-releases/releases/download/release%2Fv0.2.12-api-post3/decentralized-api-amd64.zip",
		},

		// Substring / suffix attacks
		{
			name:    "host suffix attack",
			url:     "https://github.com.evil.com/gonka-ai/gonka/releases/x.zip",
			wantErr: true,
		},
		{
			name:    "path-prefix attack (extra subdomain in path)",
			url:     "https://github.com/gonka-ai/gonka.evil.com/releases/x.zip",
			wantErr: true,
		},
		{
			name:    "raw.githubusercontent.com (wrong host)",
			url:     "https://raw.githubusercontent.com/gonka-ai/gonka/main/something",
			wantErr: true,
		},

		// Scheme attacks
		{
			name:    "http (non-TLS)",
			url:     "http://github.com/gonka-ai/gonka/releases/x.zip",
			wantErr: true,
		},
		{
			name:    "ftp",
			url:     "ftp://github.com/gonka-ai/gonka/releases/x.zip",
			wantErr: true,
		},

		// Edge cases
		{
			name:    "empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "garbled",
			url:     "://://%%%%",
			wantErr: true,
		},
		{
			name:    "user info embedded",
			url:     "https://user@github.com/gonka-ai/gonka/releases/x.zip",
			wantErr: true,
		},
		{
			name:    "wrong path prefix (similar but wrong owner)",
			url:     "https://github.com/evil/gonka/releases/x.zip",
			wantErr: true,
		},
		{
			name:    "wrong path prefix (similar repo name)",
			url:     "https://github.com/product-science/other-repo/releases/x.zip",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReleaseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReleaseURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
