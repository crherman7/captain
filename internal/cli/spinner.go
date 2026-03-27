package cli

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var spinFrames = []string{"◐", "◓", "◑", "◒"}

// Spinner renders inline status lines with an animated spinner.
type Spinner struct {
	w       io.Writer
	mu      sync.Mutex
	ticker  *time.Ticker
	done    chan struct{}
	name    string
	message string
	started time.Time
	frame   int
	active  bool
}

// NewSpinner creates a spinner that writes to w (typically os.Stderr).
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{w: w}
}

// Start begins spinning with the given service name and status message.
func (s *Spinner) Start(name, message string) {
	s.mu.Lock()
	s.name = name
	s.message = message
	s.started = time.Now()
	s.frame = 0
	s.active = true
	s.done = make(chan struct{})
	s.ticker = time.NewTicker(100 * time.Millisecond)
	s.mu.Unlock()

	s.render()

	go func() {
		for {
			select {
			case <-s.done:
				return
			case <-s.ticker.C:
				s.mu.Lock()
				s.frame++
				s.mu.Unlock()
				s.render()
			}
		}
	}()
}

// Update changes the status message while the spinner is active.
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
	s.render()
}

// Stop ends the spinner and prints the final line with icon and elapsed time.
func (s *Spinner) Stop(icon, message string) {
	s.mu.Lock()
	if s.active {
		s.ticker.Stop()
		close(s.done)
		s.active = false
	}
	elapsed := time.Since(s.started)
	name := s.name
	s.mu.Unlock()

	// Clear line and print final result
	var iconColor string
	switch icon {
	case "✔":
		iconColor = colorGreen
	case "✗":
		iconColor = colorRed
	default:
		iconColor = colorReset
	}

	fmt.Fprintf(s.w, "\033[2K\r  %s%s%s %-20s %-16s %s%s%s\n", //nolint:errcheck
		iconColor, icon, colorReset,
		name,
		message,
		colorDim, formatDuration(elapsed), colorReset,
	)
}

// Skip prints a skipped/unchanged line (no spinner, no timing).
func (s *Spinner) Skip(name, message string) {
	fmt.Fprintf(s.w, "  %s-%s %-20s %s%s%s\n", //nolint:errcheck
		colorDim, colorReset,
		name,
		colorDim, message, colorReset,
	)
}

func (s *Spinner) render() {
	s.mu.Lock()
	frame := spinFrames[s.frame%len(spinFrames)]
	name := s.name
	message := s.message
	s.mu.Unlock()

	// Truncate message to prevent line wrapping (leave room for prefix)
	const maxMsg = 50
	if len(message) > maxMsg {
		message = message[:maxMsg-3] + "..."
	}

	fmt.Fprintf(s.w, "\033[2K\r  %s%s%s %-20s %s", //nolint:errcheck
		colorCyan, frame, colorReset,
		name,
		message,
	)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
