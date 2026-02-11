package docker

import (
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func TestBaseArgs(t *testing.T) {
	c := &ComposeClient{
		Files: []string{"docker-compose.yml", "docker-compose.mlnode.yml"},
	}

	args := c.baseArgs()
	want := []string{"-f", "docker-compose.yml", "-f", "docker-compose.mlnode.yml"}

	if len(args) != len(want) {
		t.Fatalf("baseArgs() length = %d, want %d", len(args), len(want))
	}

	for i, a := range args {
		if a != want[i] {
			t.Errorf("baseArgs()[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestBaseArgs_SingleFile(t *testing.T) {
	c := &ComposeClient{
		Files: []string{"docker-compose.yml"},
	}

	args := c.baseArgs()
	if len(args) != 2 {
		t.Fatalf("baseArgs() length = %d, want 2", len(args))
	}
	if args[0] != "-f" || args[1] != "docker-compose.yml" {
		t.Errorf("baseArgs() = %v, want [-f docker-compose.yml]", args)
	}
}

func TestBaseArgs_Empty(t *testing.T) {
	c := &ComposeClient{}

	args := c.baseArgs()
	if len(args) != 0 {
		t.Errorf("baseArgs() with no files = %v, want empty", args)
	}
}

func TestNewComposeClient(t *testing.T) {
	state := config.NewState("/tmp/test-deploy")
	state.ComposeFiles = []string{"dc1.yml", "dc2.yml"}
	state.UseSudo = true

	client, err := NewComposeClient(state)
	if err != nil {
		t.Fatalf("NewComposeClient() error: %v", err)
	}

	if client.WorkDir != "/tmp/test-deploy" {
		t.Errorf("WorkDir = %q, want %q", client.WorkDir, "/tmp/test-deploy")
	}
	if len(client.Files) != 2 {
		t.Errorf("Files length = %d, want 2", len(client.Files))
	}
	if !client.UseSudo {
		t.Error("UseSudo should be true")
	}
	if client.EnvFile != "/tmp/test-deploy/config.env" {
		t.Errorf("EnvFile = %q, want %q", client.EnvFile, "/tmp/test-deploy/config.env")
	}
}

func TestNewComposeClient_Defaults(t *testing.T) {
	state := config.NewState("/tmp/test-deploy")

	client, err := NewComposeClient(state)
	if err != nil {
		t.Fatalf("NewComposeClient() error: %v", err)
	}

	// Should use default compose files
	if len(client.Files) != 2 {
		t.Errorf("default Files length = %d, want 2", len(client.Files))
	}
	if client.Files[0] != "docker-compose.yml" {
		t.Errorf("Files[0] = %q, want %q", client.Files[0], "docker-compose.yml")
	}
	if client.Files[1] != "docker-compose.mlnode.yml" {
		t.Errorf("Files[1] = %q, want %q", client.Files[1], "docker-compose.mlnode.yml")
	}
	if client.UseSudo {
		t.Error("UseSudo should be false by default")
	}
}

func TestNewComposeClient_EmptyOutputDir(t *testing.T) {
	state := config.NewState("")
	state.OutputDir = ""

	_, err := NewComposeClient(state)
	if err == nil {
		t.Error("expected error for empty output dir, got nil")
	}
}
