package build

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

var dockerStepRe = regexp.MustCompile(`^#\d+\s+(.+)`)

// ExecRunner executes commands, capturing output and optionally streaming lines
// to an OnOutput callback.
type ExecRunner struct {
	OnOutput func(string)
}

func (e *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	// If no callback, just capture everything
	if e.OnOutput == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("%s: %w\n%s", name, err, string(out))
		}
		return out, nil
	}

	// Stream stderr line-by-line to the callback, capture everything for error reporting
	var buf bytes.Buffer

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", name, err)
	}

	// Read stdout into buffer
	go func() {
		_, _ = io.Copy(&buf, stdout)
	}()

	// Scan stderr, feed lines to callback and buffer
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')

		if msg := parseBuildLine(line); msg != "" {
			e.OnOutput(msg)
		}
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), fmt.Errorf("%s: %w\n%s", name, err, buf.String())
	}

	return buf.Bytes(), nil
}

// parseBuildLine extracts a human-readable status from a docker buildx output line.
func parseBuildLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Docker buildx lines: "#12 [build 6/6] RUN go build..."
	if m := dockerStepRe.FindStringSubmatch(line); len(m) > 1 {
		msg := m[1]
		// Skip noisy lines
		if strings.HasPrefix(msg, "sha256:") || strings.HasPrefix(msg, "[auth]") {
			return ""
		}
		// Trim "DONE" / "CACHED" suffixes for cleaner display
		msg = strings.TrimSuffix(msg, " done")
		msg = strings.TrimSuffix(msg, " DONE")
		return msg
	}

	return ""
}
