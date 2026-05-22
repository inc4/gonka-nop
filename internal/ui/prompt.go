package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/AlecAivazis/survey/v2"
)

var (
	mu             sync.RWMutex
	nonInteractive bool
	overrides      = map[string]string{}
)

// SetNonInteractive enables or disables non-interactive mode.
// When enabled, prompts return overrides or defaults instead of prompting.
func SetNonInteractive(v bool) {
	mu.Lock()
	defer mu.Unlock()
	nonInteractive = v
}

// SetOverride sets a prompt override. When a prompt message contains the key
// (case-insensitive substring match), the value is returned instead of prompting.
func SetOverride(key, value string) {
	mu.Lock()
	defer mu.Unlock()
	overrides[key] = value
}

// ResetOverrides clears all overrides and disables non-interactive mode.
// Intended for use in tests.
func ResetOverrides() {
	mu.Lock()
	defer mu.Unlock()
	nonInteractive = false
	overrides = map[string]string{}
}

// findOverride returns the override value for a prompt message, if any.
func findOverride(message string) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	lower := strings.ToLower(message)
	for key, val := range overrides {
		if strings.Contains(lower, strings.ToLower(key)) {
			return val, true
		}
	}
	return "", false
}

// IsNonInteractive returns whether non-interactive mode is enabled.
func IsNonInteractive() bool {
	mu.RLock()
	defer mu.RUnlock()
	return nonInteractive
}

// Select prompts user to select from a list of options
func Select(message string, options []string) (string, error) {
	if val, ok := findOverride(message); ok {
		// Find the option that contains the override value (case-insensitive)
		lowerVal := strings.ToLower(val)
		for _, opt := range options {
			if strings.Contains(strings.ToLower(opt), lowerVal) {
				return opt, nil
			}
		}
		// Exact match fallback
		for _, opt := range options {
			if strings.EqualFold(opt, val) {
				return opt, nil
			}
		}
		return "", fmt.Errorf("override value %q does not match any option: %v", val, options)
	}
	if IsNonInteractive() {
		if len(options) > 0 {
			return options[0], nil
		}
		return "", fmt.Errorf("no options available for prompt: %s", message)
	}

	var result string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// SelectWithDefault prompts the user to select from a list of options with
// a default highlighted at the top. Behaves like Select() with respect to
// override resolution (--yes mode looks up an exact / substring-matching
// override value in the option set) and non-interactive fallback (uses
// defaultVal if present in options, else first option).
//
// When defaultVal is in options, survey highlights it as the preselected
// entry — the operator can hit Enter to accept.
func SelectWithDefault(message string, options []string, defaultVal string) (string, error) {
	if val, ok := findOverride(message); ok {
		lowerVal := strings.ToLower(val)
		for _, opt := range options {
			if strings.Contains(strings.ToLower(opt), lowerVal) {
				return opt, nil
			}
		}
		for _, opt := range options {
			if strings.EqualFold(opt, val) {
				return opt, nil
			}
		}
		return "", fmt.Errorf("override value %q does not match any option: %v", val, options)
	}
	if IsNonInteractive() {
		for _, opt := range options {
			if opt == defaultVal {
				return defaultVal, nil
			}
		}
		if len(options) > 0 {
			return options[0], nil
		}
		return "", fmt.Errorf("no options available for prompt: %s", message)
	}

	defaultExists := false
	for _, opt := range options {
		if opt == defaultVal {
			defaultExists = true
			break
		}
	}

	var result string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	if defaultExists {
		prompt.Default = defaultVal
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// Confirm prompts user for yes/no confirmation
func Confirm(message string, defaultVal bool) (bool, error) {
	if IsNonInteractive() {
		return defaultVal, nil
	}

	var result bool
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultVal,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// Input prompts user for text input
func Input(message string, defaultVal string) (string, error) {
	if val, ok := findOverride(message); ok {
		return val, nil
	}
	if IsNonInteractive() {
		if defaultVal != "" {
			return defaultVal, nil
		}
		return "", fmt.Errorf("no default value for required input: %s", message)
	}

	var result string
	prompt := &survey.Input{
		Message: message,
		Default: defaultVal,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// Password prompts user for password input (hidden)
func Password(message string) (string, error) {
	if val, ok := findOverride(message); ok {
		return val, nil
	}
	if IsNonInteractive() {
		return "", fmt.Errorf("password required but not provided via --keyring-password flag: %s", message)
	}

	var result string
	prompt := &survey.Password{
		Message: message,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// MultiSelect prompts user to select multiple options
func MultiSelect(message string, options []string) ([]string, error) {
	if IsNonInteractive() {
		return options, nil
	}

	var result []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}
