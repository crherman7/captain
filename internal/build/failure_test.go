package build

import (
	"strings"
	"testing"
)

const bakeFailureSample = `#1 [internal] load build definition from Dockerfile
#1 transferring dockerfile: 2B done
#1 DONE 0.0s

#5 [api 1/4] FROM docker.io/library/alpine:3.18
#5 DONE 0.0s

#7 [api 3/4] RUN bad-cmd
#7 0.234 /bin/sh: bad-cmd: not found
#7 ERROR: process "/bin/sh -c bad-cmd" did not complete successfully: exit code: 127
------
 > [api 3/4] RUN bad-cmd:
0.234 /bin/sh: bad-cmd: not found
------
Dockerfile:8
--------------------
   6 |
   7 |
   8 | >>> RUN bad-cmd
   9 |
--------------------
ERROR: failed to solve: process "/bin/sh -c bad-cmd" did not complete successfully: exit code: 127
`

func TestParseBuildFailures_SingleTarget(t *testing.T) {
	failures := ParseBuildFailures([]byte(bakeFailureSample))
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
	if f.DockerfileAt != "Dockerfile:8" {
		t.Errorf("DockerfileAt = %q, want %q", f.DockerfileAt, "Dockerfile:8")
	}
	if !strings.Contains(f.Snippet, ">>> RUN bad-cmd") {
		t.Errorf("Snippet missing failing line: %q", f.Snippet)
	}
	if !strings.Contains(f.Error, "exit code: 127") {
		t.Errorf("Error = %q, want substring %q", f.Error, "exit code: 127")
	}
}

const multiTargetCancelledSample = `#7 [api build 3/4] RUN bad-cmd
#7 ERROR: process "/bin/sh -c bad-cmd" did not complete successfully: exit code: 1
#9 [worker build 2/4] RUN apk add foo
#9 ERROR: context canceled
`

func TestParseBuildFailures_FiltersCancelledSiblings(t *testing.T) {
	failures := ParseBuildFailures([]byte(multiTargetCancelledSample))
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %+v", len(failures), failures)
	}
	if failures[0].Target != "api" {
		t.Errorf("Target = %q, want %q", failures[0].Target, "api")
	}
}

func TestParseBuildFailures_EmptyInput(t *testing.T) {
	if got := ParseBuildFailures(nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCollectBuildFailures_StderrTailFallback(t *testing.T) {
	dec := newProgressDecoder(nil)
	stderr := []byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock.\nIs the docker daemon running?\n")

	failures := collectBuildFailures(dec, nil, stderr, "api")
	if len(failures) != 1 {
		t.Fatalf("expected 1 synthetic failure, got %d", len(failures))
	}
	if failures[0].Target != "api" {
		t.Errorf("Target = %q, want %q", failures[0].Target, "api")
	}
	if !strings.Contains(failures[0].Snippet, "Cannot connect to the Docker daemon") {
		t.Errorf("Snippet missing daemon error: %q", failures[0].Snippet)
	}
}

func TestCollectBuildFailures_PrefersStructured(t *testing.T) {
	dec := newProgressDecoder(nil)
	dec.HandleLine(`{"vertexes":[{"digest":"a","name":"[api 1/2] RUN x","error":"exit code: 1"}]}`)
	stderr := []byte("some unrelated noise\n")

	failures := collectBuildFailures(dec, nil, stderr, "api")
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].Snippet != "" {
		t.Errorf("expected no snippet from stderr fallback, got %q", failures[0].Snippet)
	}
}

func TestAttributeFailures_FallbackTarget(t *testing.T) {
	in := []BuildFailure{{Target: "", Error: "boom"}}
	out := attributeFailures(in, "api")
	if out[0].Target != "api" {
		t.Errorf("Target = %q, want %q", out[0].Target, "api")
	}
}
