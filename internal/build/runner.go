package build

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// ExecRunner executes shell commands. Set streamFn in tests to intercept calls
// without spawning real processes.
type ExecRunner struct {
	streamFn func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

// RunStreaming executes name with args, streaming each stdout and stderr line
// to the corresponding callbacks (nil callbacks are ignored). It returns the
// full captured stdout and stderr buffers along with any execution error.
func (e *ExecRunner) RunStreaming(
	ctx context.Context,
	onStdout func(string),
	onStderr func(string),
	name string,
	args ...string,
) (stdoutAll, stderrAll []byte, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	if e.streamFn != nil {
		err := e.streamFn(ctx, name, args, &stdoutBuf, &stderrBuf)
		if onStdout != nil {
			emitLines(stdoutBuf.Bytes(), onStdout)
		}
		if onStderr != nil {
			emitLines(stderrBuf.Bytes(), onStderr)
		}
		if err != nil {
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("%s: %w", name, err)
		}
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
	}

	cmd := exec.CommandContext(ctx, name, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("creating stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting %s: %w", name, err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanLines(stdoutPipe, &stdoutBuf, onStdout)
	}()
	go func() {
		defer wg.Done()
		scanLines(stderrPipe, &stderrBuf, onStderr)
	}()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), fmt.Errorf("%s: %w", name, err)
	}
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), nil
}

// scanLines reads r line-by-line into buf, optionally invoking onLine per line.
// BuildKit's --progress=rawjson can emit long lines, so the scanner buffer is
// generously sized.
func scanLines(r io.Reader, buf *bytes.Buffer, onLine func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		buf.WriteString(line)
		buf.WriteByte('\n')
		if onLine != nil {
			onLine(line)
		}
	}
}

// emitLines splits buf on newlines and forwards each to onLine. Used by the
// test-mock path where streamFn writes pre-baked output instead of streaming.
func emitLines(buf []byte, onLine func(string)) {
	for _, line := range strings.Split(string(buf), "\n") {
		if line == "" {
			continue
		}
		onLine(line)
	}
}

// ParseTargetMessage splits a "target:message" string emitted by the rawjson
// progress decoder. Returns (target, message). If no target prefix is present,
// returns ("", original).
func ParseTargetMessage(s string) (string, string) {
	if idx := strings.Index(s, ":"); idx > 0 {
		candidate := s[:idx]
		if !strings.ContainsAny(candidate, " []") {
			return candidate, s[idx+1:]
		}
	}
	return "", s
}
