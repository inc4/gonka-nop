package phases

import "testing"

func TestParseDockerVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Standard", "Docker version 27.4.1, build abc1234", "27.4.1", false},
		{"No build", "Docker version 24.0.7", "24.0.7", false},
		{"With newline", "Docker version 27.3.1, build abc\n", "27.3.1", false},
		{"Empty", "", "", true},
		{"No version keyword", "something else", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDockerVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDockerVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDockerComposeVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Standard", "Docker Compose version v2.32.4", "v2.32.4", false},
		{"With newline", "Docker Compose version v2.21.0\n", "v2.21.0", false},
		{"Empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDockerComposeVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDockerComposeVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseNvidiaSMICSV(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantName  string
		wantMem   int
		wantArch  string
		wantErr   bool
	}{
		{
			name:      "Single RTX 3090",
			input:     "0, NVIDIA GeForce RTX 3090, 24576, 560.35.03, 0000:01:00.0",
			wantCount: 1,
			wantName:  "NVIDIA GeForce RTX 3090",
			wantMem:   24576,
			wantArch:  "sm_86",
		},
		{
			name: "4x RTX 4090",
			input: `0, NVIDIA GeForce RTX 4090, 24564, 570.133.20, 0000:01:00.0
1, NVIDIA GeForce RTX 4090, 24564, 570.133.20, 0000:02:00.0
2, NVIDIA GeForce RTX 4090, 24564, 570.133.20, 0000:03:00.0
3, NVIDIA GeForce RTX 4090, 24564, 570.133.20, 0000:04:00.0`,
			wantCount: 4,
			wantName:  "NVIDIA GeForce RTX 4090",
			wantMem:   24564,
			wantArch:  "sm_89",
		},
		{
			name: "8x H100",
			input: `0, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:01:00.0
1, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:02:00.0
2, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:03:00.0
3, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:04:00.0
4, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:05:00.0
5, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:06:00.0
6, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:07:00.0
7, NVIDIA H100 80GB HBM3, 81920, 570.195.03, 0000:08:00.0`,
			wantCount: 8,
			wantName:  "NVIDIA H100 80GB HBM3",
			wantMem:   81920,
			wantArch:  "sm_90",
		},
		{
			name:    "Empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Too few fields",
			input:   "0, NVIDIA RTX 3090, 24576",
			wantErr: true,
		},
		{
			name:    "Bad index",
			input:   "abc, NVIDIA RTX 3090, 24576, 560.35.03, 0000:01:00.0",
			wantErr: true,
		},
		{
			name:    "Bad memory",
			input:   "0, NVIDIA RTX 3090, notanumber, 560.35.03, 0000:01:00.0",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpus, err := ParseNvidiaSMICSV(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(gpus) != tt.wantCount {
				t.Fatalf("got %d GPUs, want %d", len(gpus), tt.wantCount)
			}
			if gpus[0].Name != tt.wantName {
				t.Errorf("Name = %q, want %q", gpus[0].Name, tt.wantName)
			}
			if gpus[0].MemoryMB != tt.wantMem {
				t.Errorf("MemoryMB = %d, want %d", gpus[0].MemoryMB, tt.wantMem)
			}
			if gpus[0].Architecture != tt.wantArch {
				t.Errorf("Architecture = %q, want %q", gpus[0].Architecture, tt.wantArch)
			}
		})
	}
}

func TestParseNvidiaSMICSV_Whitespace(t *testing.T) {
	input := "\n  0, NVIDIA GeForce RTX 3090, 24576, 560.35.03, 0000:01:00.0  \n\n"
	gpus, err := ParseNvidiaSMICSV(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
	if gpus[0].Name != "NVIDIA GeForce RTX 3090" {
		t.Errorf("Name = %q", gpus[0].Name)
	}
}

func TestGPUArchFromName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"NVIDIA H200 141GB", "sm_90"},
		{"NVIDIA H100 80GB HBM3", "sm_90"},
		{"NVIDIA A100 40GB", "sm_80"},
		{"NVIDIA A100-SXM4-80GB", "sm_80"},
		{"NVIDIA GeForce RTX 4090", "sm_89"},
		{"NVIDIA GeForce RTX 4080", "sm_89"},
		{"NVIDIA RTX 6000 Ada Generation", "sm_89"},
		{"NVIDIA L40", "sm_89"},
		{"NVIDIA L4", "sm_89"},
		{"NVIDIA RTX A6000", "sm_86"},
		{"NVIDIA A40", "sm_86"},
		{"NVIDIA GeForce RTX 3090", "sm_86"},
		{"NVIDIA GeForce RTX 3080", "sm_86"},
		{"NVIDIA B200", "sm_100"},
		{"NVIDIA B300", "sm_100"},
		{"NVIDIA GeForce RTX 5090", "sm_120"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GPUArchFromName(tt.name)
			if got != tt.want {
				t.Errorf("GPUArchFromName(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestGPUArchFromName_Unknown(t *testing.T) {
	got := GPUArchFromName("Some Unknown GPU")
	if got != "unknown" {
		t.Errorf("GPUArchFromName(unknown) = %q, want %q", got, "unknown")
	}
}

func TestParseDiskFreeGB(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"Normal", " Avail\n  133G\n", 133, false},
		{"Large", " Avail\n 1024G\n", 1024, false},
		{"No suffix already stripped", " Avail\n512\n", 512, false},
		{"Single line", "133G", 0, true},
		{"Empty", "", 0, true},
		{"Bad value", " Avail\n notanumber\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDiskFreeGB(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDiskFreeGB() = %d, want %d", got, tt.want)
			}
		})
	}
}
