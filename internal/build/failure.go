package build

import (
	"regexp"
	"strings"
)

// BuildFailure describes a single failed build target extracted from BuildKit
// progress output.
type BuildFailure struct {
	Target       string // image target name (e.g. "api")
	Step         string // failing command (e.g. "RUN bad-cmd")
	DockerfileAt string // location marker (e.g. "Dockerfile:8")
	Snippet      string // Dockerfile context block emitted by BuildKit
	Error        string // short error description from BuildKit
}

// BuildError wraps a build failure with structured per-target details so the UI
// can render them cleanly instead of dumping raw stderr.
type BuildError struct {
	Failures []BuildFailure
	Err      error
}

func (e *BuildError) Error() string { return e.Err.Error() }
func (e *BuildError) Unwrap() error { return e.Err }

var (
	stepHeaderRe = regexp.MustCompile(`^#(\d+)\s+\[([^\]]+)\]\s*(.*)$`)
	stepErrorRe  = regexp.MustCompile(`^#(\d+)\s+ERROR:\s*(.+)$`)
	dashesRe     = regexp.MustCompile(`^-{4,}$`)
)

// ParseBuildFailures extracts per-target build failures from BuildKit
// `--progress=plain` stderr. Returns nil when no failures are recognised.
func ParseBuildFailures(buf []byte) []BuildFailure {
	lines := strings.Split(string(buf), "\n")

	type stepInfo struct{ target, cmd string }
	steps := map[string]*stepInfo{}
	stepErrors := map[string]string{}
	var errOrder []string

	for _, line := range lines {
		if m := stepHeaderRe.FindStringSubmatch(line); m != nil {
			n := m[1]
			inside := strings.TrimSpace(m[2])
			cmd := strings.TrimSpace(m[3])
			parts := strings.Fields(inside)
			if len(parts) == 0 || parts[0] == "internal" {
				continue
			}
			info, exists := steps[n]
			if !exists {
				steps[n] = &stepInfo{target: parts[0], cmd: cmd}
			} else if cmd != "" && info.cmd == "" {
				info.cmd = cmd
			}
			continue
		}
		if m := stepErrorRe.FindStringSubmatch(line); m != nil {
			n := m[1]
			msg := strings.TrimSpace(m[2])
			if strings.Contains(msg, "context canceled") {
				continue
			}
			if _, seen := stepErrors[n]; !seen {
				errOrder = append(errOrder, n)
			}
			stepErrors[n] = msg
		}
	}

	snippets := extractSnippets(lines)

	var failures []BuildFailure
	for i, n := range errOrder {
		info := steps[n]
		if info == nil {
			continue
		}
		f := BuildFailure{
			Target: info.target,
			Step:   info.cmd,
			Error:  stepErrors[n],
		}
		if i < len(snippets) {
			f.DockerfileAt = snippets[i].at
			f.Snippet = snippets[i].block
		}
		failures = append(failures, f)
	}
	return failures
}

type snippet struct {
	at    string
	block string
}

// extractSnippets pulls the `Dockerfile:N` ... `--------------------` blocks
// BuildKit emits after a failed step.
func extractSnippets(lines []string) []snippet {
	var out []snippet
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "Dockerfile:") {
			continue
		}
		at := line
		if i+1 >= len(lines) || !dashesRe.MatchString(strings.TrimSpace(lines[i+1])) {
			continue
		}
		j := i + 2
		var block strings.Builder
		for j < len(lines) {
			inner := lines[j]
			if dashesRe.MatchString(strings.TrimSpace(inner)) {
				break
			}
			if block.Len() > 0 {
				block.WriteString("\n")
			}
			block.WriteString(inner)
			j++
		}
		out = append(out, snippet{at: at, block: block.String()})
		i = j
	}
	return out
}

// attributeFailures replaces unattributable failures with a fallback target
// name. Useful for single-target Build invocations where BuildKit may emit
// stage names rather than the captain target name.
func attributeFailures(failures []BuildFailure, fallback string) []BuildFailure {
	for i := range failures {
		if failures[i].Target == "" {
			failures[i].Target = fallback
		}
	}
	return failures
}

// stderrTailLines is how many trailing lines of stderr to include in the
// synthetic fallback failure when no structured failure could be extracted.
const stderrTailLines = 20

// collectBuildFailures extracts structured failures from a failed buildx
// invocation. Tries the rawjson decoder first, then the legacy plain-text
// parser, and finally synthesises a failure from the stderr tail so that
// daemon-down / auth / push errors (which never reach the JSON solve loop)
// still surface a useful message instead of a bare exit-status.
func collectBuildFailures(dec *progressDecoder, stdout, stderr []byte, fallbackTarget string) []BuildFailure {
	if failures := dec.Failures(); len(failures) > 0 {
		return attributeFailures(failures, fallbackTarget)
	}
	if failures := ParseBuildFailures(stderr); len(failures) > 0 {
		return attributeFailures(failures, fallbackTarget)
	}
	if failures := ParseBuildFailures(stdout); len(failures) > 0 {
		return attributeFailures(failures, fallbackTarget)
	}
	tail := tailLines(stderr, stderrTailLines)
	if tail == "" {
		tail = tailLines(stdout, stderrTailLines)
	}
	if tail == "" {
		return nil
	}
	// Target left empty when fallbackTarget is "" — signals to callers that
	// this failure is global (infrastructure / pre-build), not attributable to
	// a specific bake target.
	return []BuildFailure{{Target: fallbackTarget, Error: "build failed", Snippet: tail}}
}

// tailLines returns the last n non-empty lines of buf joined by newlines.
func tailLines(buf []byte, n int) string {
	if len(buf) == 0 || n <= 0 {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	kept := make([]string, 0, n)
	for i := len(all) - 1; i >= 0 && len(kept) < n; i-- {
		line := all[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		kept = append([]string{line}, kept...)
	}
	return strings.Join(kept, "\n")
}
