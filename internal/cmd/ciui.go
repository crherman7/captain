package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/crherman7/captain/internal/build"
)

// ciUI is a plain-text logger for CI environments and piped output.
type ciUI struct {
	w       io.Writer
	mu      sync.Mutex
	started map[string]time.Time
}

// NewCIUI creates a CI-friendly output logger.
func NewCIUI(w io.Writer) UI {
	return &ciUI{
		w:       w,
		started: make(map[string]time.Time),
	}
}

func (c *ciUI) Header(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "\n%s\n\n", msg) //nolint:errcheck
}

func (c *ciUI) ServiceStart(name, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started[name] = time.Now()
	fmt.Fprintf(c.w, "  [start] %-20s %s\n", displayName(name), message) //nolint:errcheck
}

func (c *ciUI) ServiceUpdate(name, message string) {
	// CI mode: don't spam updates, only show start/done
}

func (c *ciUI) ServiceDone(name, icon, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elapsed := ""
	if t, ok := c.started[name]; ok {
		elapsed = " (" + formatDuration(time.Since(t)) + ")"
		delete(c.started, name)
	}
	fmt.Fprintf(c.w, "  [%s] %-20s %s%s\n", icon, displayName(name), message, elapsed) //nolint:errcheck
}

func (c *ciUI) ServiceSkip(name, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "  [-] %-20s %s\n", displayName(name), message) //nolint:errcheck
}

func (c *ciUI) Error(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var be *build.BuildError
	if errors.As(err, &be) && len(be.Failures) > 0 {
		fmt.Fprintln(c.w) //nolint:errcheck
		for _, f := range be.Failures {
			label := f.Target
			if label == "" {
				label = "build"
			}
			fmt.Fprintf(c.w, "  [error] %s", label) //nolint:errcheck
			if f.DockerfileAt != "" {
				fmt.Fprintf(c.w, "  %s", f.DockerfileAt) //nolint:errcheck
			}
			fmt.Fprintln(c.w) //nolint:errcheck
			if f.Step != "" {
				fmt.Fprintf(c.w, "    step: %s\n", f.Step) //nolint:errcheck
			}
			if f.Snippet != "" {
				for _, line := range strings.Split(f.Snippet, "\n") {
					fmt.Fprintf(c.w, "    %s\n", line) //nolint:errcheck
				}
			}
			if f.Error != "" {
				fmt.Fprintf(c.w, "    %s\n", f.Error) //nolint:errcheck
			}
		}
		return
	}
	fmt.Fprintf(c.w, "\n  [error] %v\n", err) //nolint:errcheck
}

func (c *ciUI) Flush() {}
