package ui

import (
	"github.com/AlecAivazis/survey/v2"
)

// Select prompts user to select from a list of options
func Select(message string, options []string) (string, error) {
	var result string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// Confirm prompts user for yes/no confirmation
func Confirm(message string, defaultVal bool) (bool, error) {
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
	var result string
	prompt := &survey.Password{
		Message: message,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}

// MultiSelect prompts user to select multiple options
func MultiSelect(message string, options []string) ([]string, error) {
	var result []string
	prompt := &survey.MultiSelect{
		Message: message,
		Options: options,
	}
	err := survey.AskOne(prompt, &result)
	return result, err
}
