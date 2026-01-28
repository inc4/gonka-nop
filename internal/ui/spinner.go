package ui

import (
	"time"

	"github.com/briandowns/spinner"
)

// Spinner wraps briandowns/spinner for consistent styling
type Spinner struct {
	s *spinner.Spinner
}

// NewSpinner creates a new spinner with the given message
func NewSpinner(message string) *Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + message
	return &Spinner{s: s}
}

// Start starts the spinner
func (sp *Spinner) Start() {
	sp.s.Start()
}

// Stop stops the spinner
func (sp *Spinner) Stop() {
	sp.s.Stop()
}

// UpdateMessage updates the spinner message
func (sp *Spinner) UpdateMessage(message string) {
	sp.s.Suffix = " " + message
}

// StopWithSuccess stops spinner and shows success message
func (sp *Spinner) StopWithSuccess(message string) {
	sp.s.Stop()
	Success(message)
}

// StopWithError stops spinner and shows error message
func (sp *Spinner) StopWithError(message string) {
	sp.s.Stop()
	Error(message)
}

// WithSpinner runs a function with a spinner, handling success/failure
func WithSpinner(message string, fn func() error) error {
	sp := NewSpinner(message)
	sp.Start()
	err := fn()
	if err != nil {
		sp.StopWithError(message + " - failed")
		return err
	}
	sp.StopWithSuccess(message)
	return nil
}
