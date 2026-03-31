package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func printOutput(name, key, value string) {
	fmt.Fprintf(os.Stdout, "  %s.%s → %s\n", name, key, value) //nolint:errcheck
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	for i, h := range headers {
		fmt.Fprintf(w, "  %-*s", widths[i]+2, h) //nolint:errcheck
	}
	fmt.Fprintln(w) //nolint:errcheck

	for i := range headers {
		fmt.Fprintf(w, "  %s", strings.Repeat("─", widths[i])) //nolint:errcheck
	}
	fmt.Fprintln(w) //nolint:errcheck

	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				fmt.Fprintf(w, "  %-*s", widths[i]+2, cell) //nolint:errcheck
			}
		}
		fmt.Fprintln(w) //nolint:errcheck
	}
}

// displayName strips a "phase/" prefix from a key for display.
// e.g., "build/api" -> "api", "api" -> "api"
func displayName(key string) string {
	if idx := strings.Index(key, "/"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
