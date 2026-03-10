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

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantID     string
		wantVer    string
		wantFamily string
		wantErr    bool
	}{
		{
			name: "Ubuntu 22.04",
			input: `PRETTY_NAME="Ubuntu 22.04.5 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
VERSION="22.04.5 LTS (Jammy Jellyfish)"
ID=ubuntu
ID_LIKE=debian`,
			wantID: "ubuntu", wantVer: "22.04", wantFamily: "debian",
		},
		{
			name: "Debian 12",
			input: `ID=debian
VERSION_ID="12"
ID_LIKE=""`,
			wantID: "debian", wantVer: "12", wantFamily: "debian",
		},
		{
			name: "CentOS 9",
			input: `ID="centos"
VERSION_ID="9"
ID_LIKE="rhel fedora"`,
			wantID: "centos", wantVer: "9", wantFamily: "rhel",
		},
		{
			name: "Rocky Linux",
			input: `ID="rocky"
VERSION_ID="9.3"
ID_LIKE="rhel centos fedora"`,
			wantID: "rocky", wantVer: "9.3", wantFamily: "rhel",
		},
		{
			name: "Amazon Linux",
			input: `ID="amzn"
VERSION_ID="2023"`,
			wantID: "amzn", wantVer: "2023", wantFamily: "rhel",
		},
		{
			name:    "Empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "No ID",
			input:   "VERSION_ID=22.04",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ParseOSRelease(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if d.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", d.ID, tt.wantID)
			}
			if d.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", d.Version, tt.wantVer)
			}
			if d.Family != tt.wantFamily {
				t.Errorf("Family = %q, want %q", d.Family, tt.wantFamily)
			}
		})
	}
}

func TestParseModinfoVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Normal", "filename:       /lib/modules/5.15.0/nvidia.ko\nversion:        570.133.20\nsrcversion:     abc123", "570.133.20"},
		{"Extra spaces", "version:   560.35.03  ", "560.35.03"},
		{"No version line", "filename: /lib/modules/nvidia.ko\nsrcversion: abc", ""},
		{"Empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModinfoVersion(tt.input)
			if got != tt.want {
				t.Errorf("ParseModinfoVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseFabricManagerVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Installed", "ii  nvidia-fabricmanager-570  570.133.20-1  amd64  NVIDIA Fabric Manager", "570.133.20-1"},
		{"Not installed", "dpkg-query: no packages found matching nvidia-fabricmanager*", ""},
		{"Empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFabricManagerVersion(tt.input)
			if got != tt.want {
				t.Errorf("ParseFabricManagerVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDriverMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"570.133.20", "570"},
		{"560.35.03", "560"},
		{"535", "535"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DriverMajorVersion(tt.input)
			if got != tt.want {
				t.Errorf("DriverMajorVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseLsblkJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{
			name: "Mixed mounted and unmounted drives",
			input: `{
				"blockdevices": [
					{"name":"nvme0n1","size":480103981056,"type":"disk","mountpoint":null,"fstype":null,"children":[
						{"name":"nvme0n1p1","size":536870912,"type":"part","mountpoint":"/boot/efi","fstype":"vfat"},
						{"name":"nvme0n1p2","size":479565127680,"type":"part","mountpoint":"/","fstype":"ext4"}
					]},
					{"name":"nvme1n1","size":3840755982336,"type":"disk","mountpoint":"/GONKA","fstype":"ext4"},
					{"name":"nvme2n1","size":3840755982336,"type":"disk","mountpoint":null,"fstype":null},
					{"name":"nvme3n1","size":3840755982336,"type":"disk","mountpoint":null,"fstype":null},
					{"name":"loop0","size":113246208,"type":"loop","mountpoint":"/snap/core","fstype":"squashfs"}
				]
			}`,
			wantCount: 5,
		},
		{
			name:      "Empty blockdevices",
			input:     `{"blockdevices": []}`,
			wantCount: 0,
		},
		{
			name:    "Invalid JSON",
			input:   `not json`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices, err := ParseLsblkJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(devices) != tt.wantCount {
				t.Errorf("got %d devices, want %d", len(devices), tt.wantCount)
			}
		})
	}
}

func TestParseLsblkJSON_Children(t *testing.T) {
	input := `{
		"blockdevices": [
			{"name":"nvme0n1","size":480103981056,"type":"disk","mountpoint":null,"fstype":null,"children":[
				{"name":"nvme0n1p1","size":536870912,"type":"part","mountpoint":"/boot/efi","fstype":"vfat"},
				{"name":"nvme0n1p2","size":479565127680,"type":"part","mountpoint":"/","fstype":"ext4"}
			]}
		]
	}`
	devices, err := ParseLsblkJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if len(devices[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(devices[0].Children))
	}
}

func TestFindUnmountedDrives(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	tests := []struct {
		name      string
		devices   []BlockDevice
		minSizeGB int
		wantCount int
		wantNames []string
	}{
		{
			name: "Mix of mounted and unmounted",
			devices: []BlockDevice{
				{Name: "nvme0n1", Size: 480e9, Type: "disk", Mountpoint: nil, Children: []BlockDevice{
					{Name: "nvme0n1p1", Size: 500e6, Type: "part", Mountpoint: strPtr("/boot/efi")},
					{Name: "nvme0n1p2", Size: 479e9, Type: "part", Mountpoint: strPtr("/")},
				}},
				{Name: "nvme1n1", Size: 3840e9, Type: "disk", Mountpoint: strPtr("/GONKA")},
				{Name: "nvme2n1", Size: 3840e9, Type: "disk", Mountpoint: nil},
				{Name: "nvme3n1", Size: 3840e9, Type: "disk", Mountpoint: nil},
			},
			minSizeGB: 500,
			wantCount: 2,
			wantNames: []string{"nvme2n1", "nvme3n1"},
		},
		{
			name: "Loop devices excluded",
			devices: []BlockDevice{
				{Name: "loop0", Size: 1e12, Type: "loop", Mountpoint: nil},
				{Name: "nvme2n1", Size: 3840e9, Type: "disk", Mountpoint: nil},
			},
			minSizeGB: 500,
			wantCount: 1,
			wantNames: []string{"nvme2n1"},
		},
		{
			name: "Drive too small",
			devices: []BlockDevice{
				{Name: "sda", Size: 100e9, Type: "disk", Mountpoint: nil},
			},
			minSizeGB: 500,
			wantCount: 0,
		},
		{
			name: "Drive with mounted partition excluded",
			devices: []BlockDevice{
				{Name: "nvme0n1", Size: 3840e9, Type: "disk", Mountpoint: nil, Children: []BlockDevice{
					{Name: "nvme0n1p1", Size: 3840e9, Type: "part", Mountpoint: strPtr("/data")},
				}},
			},
			minSizeGB: 500,
			wantCount: 0,
		},
		{
			name: "All drives mounted",
			devices: []BlockDevice{
				{Name: "nvme1n1", Size: 3840e9, Type: "disk", Mountpoint: strPtr("/data")},
				{Name: "nvme2n1", Size: 3840e9, Type: "disk", Mountpoint: strPtr("/backup")},
			},
			minSizeGB: 500,
			wantCount: 0,
		},
		{
			name: "Drive with existing filesystem but unmounted",
			devices: []BlockDevice{
				{Name: "nvme4n1", Size: 3840e9, Type: "disk", Mountpoint: nil, Fstype: strPtr("ext4")},
			},
			minSizeGB: 500,
			wantCount: 1,
			wantNames: []string{"nvme4n1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindUnmountedDrives(tt.devices, tt.minSizeGB)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d drives, want %d", len(got), tt.wantCount)
			}
			for i, name := range tt.wantNames {
				if i < len(got) && got[i].Name != name {
					t.Errorf("drive[%d].Name = %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}

func TestParseDfSource(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"Normal", "Filesystem\n/dev/nvme0n1p3\n", "/dev/nvme0n1p3", false},
		{"With extra whitespace", "  Filesystem  \n  /dev/sda1  \n", "/dev/sda1", false},
		{"Tmpfs", "Filesystem\ntmpfs\n", "tmpfs", false},
		{"Single line", "/dev/sda1", "", true},
		{"Empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDfSource(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDfSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDriveSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{3840755982336, "3.8 TB"},
		{480103981056, "480 GB"},
		{1000000000000, "1.0 TB"},
		{500000000000, "500 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDriveSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatDriveSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
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
