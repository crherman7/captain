package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

func printHeader(msg string) {
	fmt.Fprintf(os.Stderr, "\n%s%s%s\n\n", colorBold, msg, colorReset) //nolint:errcheck
}

func printError(err error) {
	fmt.Fprintf(os.Stderr, "\n%s✗ Error:%s %v\n", colorRed, colorReset, err) //nolint:errcheck
}

func printOutput(name, key, value string) {
	fmt.Fprintf(os.Stdout, "  %s%s%s.%s → %s\n", colorCyan, name, colorReset, key, value) //nolint:errcheck
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
