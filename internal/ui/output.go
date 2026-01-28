package ui

import (
	"fmt"

	"github.com/fatih/color"
)

var (
	cyan    = color.New(color.FgCyan)
	green   = color.New(color.FgGreen)
	yellow  = color.New(color.FgYellow)
	red     = color.New(color.FgRed)
	bold    = color.New(color.Bold)
	dimmed  = color.New(color.Faint)
)

// Info prints an informational message
func Info(format string, args ...interface{}) {
	cyan.Print("ℹ ")
	fmt.Printf(format+"\n", args...)
}

// Success prints a success message
func Success(format string, args ...interface{}) {
	green.Print("✓ ")
	fmt.Printf(format+"\n", args...)
}

// Warn prints a warning message
func Warn(format string, args ...interface{}) {
	yellow.Print("⚠ ")
	fmt.Printf(format+"\n", args...)
}

// Error prints an error message
func Error(format string, args ...interface{}) {
	red.Print("✗ ")
	fmt.Printf(format+"\n", args...)
}

// Header prints a section header
func Header(text string) {
	fmt.Println()
	bold.Println(text)
	dimmed.Println(repeat("─", len(text)))
}

// Phase prints phase start indicator
func PhaseStart(number int, name string) {
	fmt.Println()
	cyan.Printf("━━━ Phase %d: %s ", number, name)
	dimmed.Println(repeat("━", 40-len(name)))
}

// PhaseComplete prints phase completion
func PhaseComplete(name string) {
	green.Printf("✓ %s complete\n", name)
}

// PhaseFailed prints phase failure
func PhaseFailed(name string, err error) {
	red.Printf("✗ %s failed: %v\n", name, err)
}

// Detail prints indented detail line
func Detail(format string, args ...interface{}) {
	dimmed.Print("  → ")
	fmt.Printf(format+"\n", args...)
}

// repeat returns a string repeated n times
func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
