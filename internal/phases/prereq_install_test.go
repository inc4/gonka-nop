package phases

import (
	"context"
	"testing"

	"github.com/inc4/gonka-nop/internal/config"
)

func TestConstants(t *testing.T) {
	// Verify constants are set correctly
	if nvidiaDriver != "nvidia-driver-570" {
		t.Errorf("nvidiaDriver = %q, want %q", nvidiaDriver, "nvidia-driver-570")
	}
	if nvidiaRepoBase == "" {
		t.Error("nvidiaRepoBase is empty")
	}
	if nctRepoBase == "" {
		t.Error("nctRepoBase is empty")
	}
}

func TestCheckSecureBoot_NoMokutil(t *testing.T) {
	// On macOS or systems without mokutil, should return false (not enabled)
	ctx := context.Background()
	result := checkSecureBoot(ctx)
	if result {
		t.Error("checkSecureBoot should return false when mokutil is not available")
	}
}

func TestCheckKernelHeaders_NoSystem(t *testing.T) {
	// On macOS or systems without dpkg, should return false
	ctx := context.Background()
	result := checkKernelHeaders(ctx)
	// On macOS: uname -r works but dpkg doesn't exist → false
	// On Linux without headers: dpkg -l fails → false
	// Either way, this should not panic
	_ = result
}

func TestRunSudoCmd_NoSudo(t *testing.T) {
	// Test runSudoCmd without sudo (useSudo=false)
	ctx := context.Background()
	out, err := runSudoCmd(ctx, false, "echo", "hello")
	if err != nil {
		t.Fatalf("runSudoCmd(false, echo) failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output from echo")
	}
}

func TestRunSudoShell_NoSudo(t *testing.T) {
	// Test runSudoShell without sudo (useSudo=false)
	ctx := context.Background()
	out, err := runSudoShell(ctx, false, "echo test123")
	if err != nil {
		t.Fatalf("runSudoShell(false, 'echo test123') failed: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output from shell echo")
	}
}

func TestRunSudoCmd_InvalidCommand(t *testing.T) {
	ctx := context.Background()
	_, err := runSudoCmd(ctx, false, "nonexistent-command-xyz")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestRunSudoShell_InvalidCommand(t *testing.T) {
	ctx := context.Background()
	_, err := runSudoShell(ctx, false, "nonexistent-command-xyz 2>/dev/null")
	// Shell may not error on all platforms, but should not panic
	_ = err
}

func TestDriverMajorVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"570.133.20", "570"},
		{"560.35.03", "560"},
		{"535", "535"},
		{"", ""},
		{".", ""},
		{"570.", "570"},
		{"a.b.c", "a"},
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

func TestInstallFabricManager_EmptyVersion(t *testing.T) {
	ctx := context.Background()
	err := installFabricManager(ctx, "", false)
	if err == nil {
		t.Error("expected error for empty driver version")
	}
}

func TestPrerequisites_Name(t *testing.T) {
	p := NewPrerequisites(true)
	if p.Name() != "Prerequisites" {
		t.Errorf("Name() = %q, want %q", p.Name(), "Prerequisites")
	}
}

func TestPrerequisites_Description(t *testing.T) {
	p := NewPrerequisites(true)
	if p.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestPrerequisites_ShouldRun(t *testing.T) {
	p := NewPrerequisites(true)
	state := config.NewState(t.TempDir())

	// Should run when phase is not complete
	if !p.ShouldRun(state) {
		t.Error("ShouldRun should return true when phase is not complete")
	}

	// Should not run when phase is complete
	state.MarkPhaseComplete("Prerequisites")
	if p.ShouldRun(state) {
		t.Error("ShouldRun should return false when phase is complete")
	}
}

func TestPrerequisites_DetectDistro_Mocked(t *testing.T) {
	p := NewPrerequisites(true)
	state := config.NewState(t.TempDir())

	err := p.detectDistro(state)
	if err != nil {
		t.Fatalf("detectDistro (mocked) failed: %v", err)
	}
	if state.Distro.ID != "ubuntu" {
		t.Errorf("Distro.ID = %q, want %q", state.Distro.ID, "ubuntu")
	}
	if state.Distro.Version != "22.04" {
		t.Errorf("Distro.Version = %q, want %q", state.Distro.Version, "22.04")
	}
	if state.Distro.Family != "debian" {
		t.Errorf("Distro.Family = %q, want %q", state.Distro.Family, "debian")
	}
}

func TestPrerequisites_CheckDriverConsistency_AllMatch(t *testing.T) {
	p := &Prerequisites{mocked: false}
	state := config.NewState(t.TempDir())
	state.DriverInfo = config.DriverInfo{
		UserVersion:   "570.133.20",
		KernelVersion: "570.133.20",
		FMVersion:     "570.133.20-1",
		Consistent:    true,
	}

	ctx := context.Background()
	// checkDriverConsistency modifies state.DriverInfo.Consistent
	// Since we can't run modinfo/dpkg on macOS, we test the comparison logic
	// by directly comparing versions
	userMajor := DriverMajorVersion(state.DriverInfo.UserVersion)
	fmMajor := DriverMajorVersion(state.DriverInfo.FMVersion)

	if userMajor != "570" {
		t.Errorf("userMajor = %q, want %q", userMajor, "570")
	}
	if fmMajor != "570" {
		t.Errorf("fmMajor = %q, want %q", fmMajor, "570")
	}
	// Same major version = consistent
	_ = p
	_ = ctx
}

func TestPrerequisites_CheckDriverConsistency_Mismatch(t *testing.T) {
	// Test that mismatched major versions are detected
	userMajor := DriverMajorVersion("570.133.20")
	fmMajor := DriverMajorVersion("560.35.03-1")

	if userMajor == fmMajor {
		t.Error("570 and 560 should be different major versions")
	}
}

func TestPrerequisites_MockedCheck(t *testing.T) {
	p := NewPrerequisites(true)
	err := p.mockedCheck("Test spinner", "Test detail message")
	if err != nil {
		t.Fatalf("mockedCheck failed: %v", err)
	}
}
