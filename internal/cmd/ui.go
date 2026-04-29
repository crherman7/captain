package cmd

import (
	"os"

	"golang.org/x/term"
)

// UI abstracts the output layer so captain works in both interactive terminals
// (multi-line spinners via bubbletea) and CI environments (plain log lines).
type UI interface {
	Header(msg string)
	ServiceStart(name, message string)
	ServiceUpdate(name, message string)
	ServiceDone(name, icon, message string)
	ServiceSkip(name, message string)
	Error(err error)
	Flush()
}

// NewUI returns a TUI for interactive terminals, or a plain CI logger otherwise.
func NewUI() UI {
	if isInteractive() {
		return NewTUI()
	}
	return NewCIUI(os.Stderr)
}

func isInteractive() bool {
	if os.Getenv("CI") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}
