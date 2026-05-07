package build

import (
	"encoding/base64"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestProgressDecoder_FailingVertex(t *testing.T) {
	dec := newProgressDecoder(nil)

	dec.HandleLine(`{"vertexes":[{"digest":"sha256:abc","name":"[api 3/4] RUN bad-cmd","started":"2024-01-01T00:00:00Z"}]}`)
	dec.HandleLine(`{"logs":[{"vertex":"sha256:abc","stream":2,"data":"` + b64("/bin/sh: bad-cmd: not found\n") + `"}]}`)
	dec.HandleLine(`{"vertexes":[{"digest":"sha256:abc","name":"[api 3/4] RUN bad-cmd","completed":"2024-01-01T00:00:01Z","error":"process \"/bin/sh -c bad-cmd\" did not complete successfully: exit code: 127"}]}`)

	failures := dec.Failures()
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	f := failures[0]
	if f.Target != "api" {
		t.Errorf("Target = %q, want %q", f.Target, "api")
	}
	if f.Step != "RUN bad-cmd" {
		t.Errorf("Step = %q, want %q", f.Step, "RUN bad-cmd")
	}
	if !strings.Contains(f.Error, "exit code: 127") {
		t.Errorf("Error = %q, want substring %q", f.Error, "exit code: 127")
	}
	if !strings.Contains(f.Snippet, "bad-cmd: not found") {
		t.Errorf("Snippet = %q, want substring %q", f.Snippet, "bad-cmd: not found")
	}
}

func TestProgressDecoder_FiltersCancelledSiblings(t *testing.T) {
	dec := newProgressDecoder(nil)

	dec.HandleLine(`{"vertexes":[{"digest":"a","name":"[api 3/4] RUN bad","error":"exit code: 1"}]}`)
	dec.HandleLine(`{"vertexes":[{"digest":"b","name":"[worker 2/4] RUN apk add foo","error":"context canceled"}]}`)

	failures := dec.Failures()
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %+v", len(failures), failures)
	}
	if failures[0].Target != "api" {
		t.Errorf("Target = %q, want %q", failures[0].Target, "api")
	}
}

func TestProgressDecoder_SkipsInternalAndUnnamed(t *testing.T) {
	dec := newProgressDecoder(nil)
	dec.HandleLine(`{"vertexes":[{"digest":"a","name":"[internal] load build definition","error":"x"}]}`)
	dec.HandleLine(`{"vertexes":[{"digest":"b","name":"resolve image config","error":"y"}]}`)

	if got := dec.Failures(); len(got) != 0 {
		t.Errorf("expected 0 failures, got %+v", got)
	}
}

func TestProgressDecoder_EmitsLiveMessages(t *testing.T) {
	var msgs []string
	dec := newProgressDecoder(func(m string) { msgs = append(msgs, m) })

	dec.HandleLine(`{"vertexes":[{"digest":"a","name":"[api 3/4] RUN echo","started":"2024-01-01T00:00:00Z"}]}`)
	dec.HandleLine(`{"vertexes":[{"digest":"b","name":"[api 4/4] COPY .","cached":true,"completed":"2024-01-01T00:00:01Z"}]}`)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d: %v", len(msgs), msgs)
	}
	if !strings.HasPrefix(msgs[0], "api:") {
		t.Errorf("msg[0] = %q, want api: prefix", msgs[0])
	}
	if !strings.Contains(msgs[1], "cached") {
		t.Errorf("msg[1] = %q, want cached substring", msgs[1])
	}
}

func TestProgressDecoder_IgnoresGarbage(t *testing.T) {
	dec := newProgressDecoder(nil)
	dec.HandleLine(`WARNING: some non-json text from stderr that leaked`)
	dec.HandleLine(`{"vertexes":[{"digest":"a","name":"[api 1/2] FROM alpine","error":"exit code: 1"}]}`)

	if len(dec.Failures()) != 1 {
		t.Errorf("expected 1 failure (garbage line should be ignored)")
	}
}

func TestSplitVertexName(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantTgt   string
		wantStep  string
	}{
		{"bake form", "[api build 3/4] RUN bad-cmd", "api", "RUN bad-cmd"},
		{"plain form", "[api 3/4] RUN echo", "api", "RUN echo"},
		{"internal", "[internal] load build definition", "internal", "load build definition"},
		{"no brackets", "load .dockerignore", "", "load .dockerignore"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tgt, step := splitVertexName(tt.in)
			if tgt != tt.wantTgt || step != tt.wantStep {
				t.Errorf("splitVertexName(%q) = (%q,%q), want (%q,%q)", tt.in, tgt, step, tt.wantTgt, tt.wantStep)
			}
		})
	}
}
