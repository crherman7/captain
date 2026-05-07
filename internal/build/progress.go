package build

import (
	"encoding/json"
	"regexp"
	"strings"
)

// solveStatus mirrors the BuildKit client.SolveStatus JSON shape emitted by
// `docker buildx ... --progress=rawjson`. Only fields we consume are decoded;
// unknown fields are ignored.
type solveStatus struct {
	Vertexes []rawVertex    `json:"vertexes,omitempty"`
	Statuses []rawStatus    `json:"statuses,omitempty"`
	Logs     []rawVertexLog `json:"logs,omitempty"`
}

type rawVertex struct {
	Digest    string `json:"digest"`
	Name      string `json:"name"`
	Cached    bool   `json:"cached,omitempty"`
	Started   string `json:"started,omitempty"`
	Completed string `json:"completed,omitempty"`
	Error     string `json:"error,omitempty"`
}

type rawStatus struct {
	Vertex string `json:"vertex"`
	Name   string `json:"name,omitempty"`
}

type rawVertexLog struct {
	Vertex string `json:"vertex"`
	Stream int    `json:"stream"`
	Data   []byte `json:"data"` // base64-decoded automatically
}

// vertexNameRe matches the bracketed prefix in a BuildKit vertex name, e.g.
// "[api build 3/4] RUN bad-cmd". The first word inside the brackets is the
// bake target (or the docker stage name for plain builds).
var vertexNameRe = regexp.MustCompile(`^\[([a-zA-Z0-9_.-]+)([^\]]*)\]\s*(.*)$`)

// progressDecoder consumes BuildKit rawjson lines, fans live status messages
// out to a callback, and accumulates per-vertex state for failure reporting.
// Not safe for concurrent use; RunStreaming feeds it from a single goroutine.
type progressDecoder struct {
	onMessage func(string) // optional UI callback in "target:message" form
	vertexes  map[string]*vertexState
	order     []string // digest order, for stable failure ordering
}

type vertexState struct {
	Name   string
	Target string
	Step   string
	Cached bool
	Error  string
	Logs   []string // tail of decoded log data
}

const maxLogTail = 20

func newProgressDecoder(onMessage func(string)) *progressDecoder {
	return &progressDecoder{
		onMessage: onMessage,
		vertexes:  make(map[string]*vertexState),
	}
}

// HandleLine decodes one rawjson line and updates state. Non-JSON lines (e.g.
// occasional warning text BuildKit leaks onto stdout) are ignored.
func (p *progressDecoder) HandleLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ss solveStatus
	if err := json.Unmarshal([]byte(line), &ss); err != nil {
		return
	}

	for _, v := range ss.Vertexes {
		st, ok := p.vertexes[v.Digest]
		if !ok {
			st = &vertexState{}
			p.vertexes[v.Digest] = st
			p.order = append(p.order, v.Digest)
		}
		if v.Name != "" {
			st.Name = v.Name
			target, step := splitVertexName(v.Name)
			st.Target = target
			st.Step = step
		}
		if v.Cached {
			st.Cached = true
		}
		if v.Error != "" {
			st.Error = v.Error
		}
		p.maybeEmit(v, st)
	}

	for _, l := range ss.Logs {
		st, ok := p.vertexes[l.Vertex]
		if !ok {
			continue
		}
		if st.Logs == nil {
			st.Logs = make([]string, 0, maxLogTail)
		}
		for _, line := range strings.Split(strings.TrimRight(string(l.Data), "\n"), "\n") {
			if line == "" {
				continue
			}
			if len(st.Logs) == maxLogTail {
				copy(st.Logs, st.Logs[1:])
				st.Logs[maxLogTail-1] = line
			} else {
				st.Logs = append(st.Logs, line)
			}
		}
	}
}

// maybeEmit calls onMessage with a "target:message" string for UI updates,
// matching the contract of the legacy parseBuildLine path.
func (p *progressDecoder) maybeEmit(v rawVertex, st *vertexState) {
	if p.onMessage == nil || st.Target == "" || st.Target == "internal" {
		return
	}
	switch {
	case v.Cached:
		p.onMessage(st.Target + ":cached " + st.Step)
	case v.Error != "":
		// failure surfaced separately; don't spam UI
	case v.Completed != "":
		p.onMessage(st.Target + ":" + st.Step)
	case v.Started != "":
		p.onMessage(st.Target + ":" + st.Step)
	}
}

// Failures returns one BuildFailure per errored vertex. Cancellations from
// sibling failures are filtered out.
func (p *progressDecoder) Failures() []BuildFailure {
	var out []BuildFailure
	for _, digest := range p.order {
		st := p.vertexes[digest]
		if st.Error == "" {
			continue
		}
		if strings.Contains(st.Error, "context canceled") {
			continue
		}
		if st.Target == "" || st.Target == "internal" {
			continue
		}
		f := BuildFailure{
			Target: st.Target,
			Step:   st.Step,
			Error:  st.Error,
		}
		if len(st.Logs) > 0 {
			f.Snippet = strings.Join(st.Logs, "\n")
		}
		out = append(out, f)
	}
	return out
}

// splitVertexName extracts (target, step) from a BuildKit vertex name.
// "[api build 3/4] RUN bad-cmd" -> ("api", "RUN bad-cmd")
// "[internal] load build definition" -> ("internal", "load build definition")
// "load .dockerignore" -> ("", "load .dockerignore")
func splitVertexName(name string) (target, step string) {
	m := vertexNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", strings.TrimSpace(name)
	}
	target = m[1]
	step = strings.TrimSpace(m[3])
	if step == "" {
		// fall back to whatever was inside the brackets after the target name
		step = strings.TrimSpace(m[2])
	}
	return target, step
}
