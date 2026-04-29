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

var (
	dockerStepRe   = regexp.MustCompile(`^#\d+\s+(.+)`)
	dockerTargetRe = regexp.MustCompile(`\[([a-zA-Z0-9_-]+)\s+`)
)

// ExecRunner executes shell commands.
// Set runFn to intercept calls in tests without spawning real processes.
type ExecRunner struct {
	runFn func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Run executes the named command, streaming parsed output lines to onOutput when non-nil.
// If runFn is set it is called instead of exec (onOutput is ignored in that case).
func (e *ExecRunner) Run(ctx context.Context, onOutput func(string), name string, args ...string) ([]byte, error) {
	if e.runFn != nil {
		return e.runFn(ctx, name, args...)
	}

	cmd := exec.CommandContext(ctx, name, args...)

	if onOutput == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("%s: %w\n%s", name, err, string(out))
		}
		return out, nil
	}

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

	go func() {
		_, _ = io.Copy(&buf, stdout)
	}()

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')

		if msg := parseBuildLine(line); msg != "" {
			onOutput(msg)
		}
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), fmt.Errorf("%s: %w\n%s", name, err, buf.String())
	}

	return buf.Bytes(), nil
}

// parseBuildLine extracts a human-readable status from a docker buildx output line.
// For bake, the target name is embedded in the step: "[api build 6/6] RUN ..."
// Returns "target:message" if a target is found, or just "message" otherwise.
func parseBuildLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	if m := dockerStepRe.FindStringSubmatch(line); len(m) > 1 {
		msg := m[1]
		if strings.HasPrefix(msg, "sha256:") || strings.HasPrefix(msg, "[auth]") {
			return ""
		}
		msg = strings.TrimSuffix(msg, " done")
		msg = strings.TrimSuffix(msg, " DONE")

		// Extract target name from "[targetname step]" pattern
		if tm := dockerTargetRe.FindStringSubmatch(msg); len(tm) > 1 {
			target := tm[1]
			// Skip internal docker stages like "internal"
			if target != "internal" {
				return target + ":" + msg
			}
		}

		return msg
	}

	return ""
}

// ParseTargetMessage splits a "target:message" string from parseBuildLine.
// Returns (target, message). If no target prefix, returns ("", original).
func ParseTargetMessage(s string) (string, string) {
	if idx := strings.Index(s, ":"); idx > 0 {
		candidate := s[:idx]
		// Only treat as target if it's a simple name (no spaces, no brackets)
		if !strings.ContainsAny(candidate, " []") {
			return candidate, s[idx+1:]
		}
	}
	return "", s
}
