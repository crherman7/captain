package build

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestProgressDecoder_RealRawjsonFixture replays a recorded
// `docker buildx build --progress=rawjson` stream and asserts the fields
// captain decodes are still populated.
//
// This is a drift-canary: if a future BuildKit release renames or drops
// fields in solveStatus / rawVertex, the assertions below will fail and
// flag that captain's decoder needs updating before we ship against that
// buildkit version.
//
// To refresh the fixture:
//
//	mkdir /tmp/fx && cd /tmp/fx
//	printf 'FROM alpine:3.20\nRUN echo hello > /tmp/x\n' > Dockerfile
//	docker buildx build --progress=rawjson --no-cache -t fx:tmp . \
//	  2> internal/build/testdata/rawjson_alpine_build.jsonl
func TestProgressDecoder_RealRawjsonFixture(t *testing.T) {
	f, err := os.Open("testdata/rawjson_alpine_build.jsonl")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	var emitted []string
	dec := newProgressDecoder(func(m string) {
		emitted = append(emitted, m)
	})

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lines := 0
	for scanner.Scan() {
		dec.HandleLine(scanner.Text())
		lines++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if lines == 0 {
		t.Fatal("fixture is empty")
	}

	if len(dec.vertexes) == 0 {
		t.Fatal("no vertexes decoded — solveStatus.vertexes field may have moved or renamed")
	}

	// At least one vertex should be a real build step ([N/M] prefix) and yield
	// a non-empty Step. Anything less means splitVertexName broke or vertex
	// names stopped using the bracketed format.
	var sawStep bool
	for _, st := range dec.vertexes {
		if st.Step != "" && !strings.HasPrefix(st.Name, "[internal]") {
			sawStep = true
			break
		}
	}
	if !sawStep {
		t.Fatal("no decoded vertex had a non-empty Step — vertex Name format may have changed")
	}
}
