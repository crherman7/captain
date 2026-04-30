package state

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := New()
	s.Update("api", ServiceState{
		Hash:       "abc123",
		DeployedAt: time.Now().Truncate(time.Second),
		ImageTag:   "api:latest",
	})

	if err := s.Save(path); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	ss, ok := loaded.Get("api")
	if !ok {
		t.Fatal("expected api in loaded state")
	}
	if ss.Hash != "abc123" {
		t.Errorf("hash = %q, want %q", ss.Hash, "abc123")
	}
	if ss.ImageTag != "api:latest" {
		t.Errorf("imageTag = %q, want %q", ss.ImageTag, "api:latest")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	s, err := Load("/nonexistent/path/state.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Services) != 0 {
		t.Errorf("expected empty state, got %d services", len(s.Services))
	}
}

func TestHasChanged(t *testing.T) {
	s := New()
	s.Update("api", ServiceState{Hash: "abc123"})

	if !s.HasChanged("api", "def456") {
		t.Error("expected changed for different hash")
	}
	if s.HasChanged("api", "abc123") {
		t.Error("expected unchanged for same hash")
	}
	if !s.HasChanged("unknown", "abc123") {
		t.Error("expected changed for unknown service")
	}
}

func TestUpdateConcurrent(t *testing.T) {
	s := New()

	const services = 64

	var wg sync.WaitGroup
	wg.Add(services)

	for i := range services {
		go func(i int) {
			defer wg.Done()

			name := "svc-" + strconv.Itoa(i)
			s.Update(name, ServiceState{
				Hash:       "hash-" + strconv.Itoa(i),
				DeployedAt: time.Now(),
			})
		}(i)
	}

	wg.Wait()

	for i := range services {
		name := "svc-" + strconv.Itoa(i)
		if _, ok := s.Get(name); !ok {
			t.Fatalf("missing service %q after concurrent updates", name)
		}
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	values := map[string]interface{}{
		"a": "1",
		"b": "2",
	}

	h1, err := ComputeHash(values, "tag1", "chart1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h2, err := ComputeHash(values, "tag1", "chart1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
}

func TestComputeHash_DifferentInputs(t *testing.T) {
	v1 := map[string]interface{}{"a": "1"}
	v2 := map[string]interface{}{"a": "2"}

	h1, _ := ComputeHash(v1, "tag", "chart")
	h2, _ := ComputeHash(v2, "tag", "chart")

	if h1 == h2 {
		t.Error("expected different hashes for different values")
	}

	h3, _ := ComputeHash(v1, "tag1", "chart")
	h4, _ := ComputeHash(v1, "tag2", "chart")

	if h3 == h4 {
		t.Error("expected different hashes for different image tags")
	}

	h5, _ := ComputeHash(v1, "tag", "chart1")
	h6, _ := ComputeHash(v1, "tag", "chart2")

	if h5 == h6 {
		t.Error("expected different hashes for different chart hashes")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file %s: %v", path, err)
	}
}

func TestHashBuildContext_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main")
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module test")

	h1, err := HashBuildContext(dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h2, err := HashBuildContext(dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
}

func TestHashBuildContext_ChangesOnFileEdit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	writeTestFile(t, file, "package main // v1")
	h1, _ := HashBuildContext(dir, "", nil)

	writeTestFile(t, file, "package main // v2")
	h2, _ := HashBuildContext(dir, "", nil)

	if h1 == h2 {
		t.Error("expected different hashes after file edit")
	}
}

func TestHashBuildContext_ChangesOnNewFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main")
	h1, _ := HashBuildContext(dir, "", nil)

	writeTestFile(t, filepath.Join(dir, "util.go"), "package main")
	h2, _ := HashBuildContext(dir, "", nil)

	if h1 == h2 {
		t.Error("expected different hashes after adding a file")
	}
}

func TestHashBuildContext_WatchPaths(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "apps", "api")
	pkgDir := filepath.Join(dir, "packages", "db")
	otherDir := filepath.Join(dir, "apps", "web")

	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(appDir, "main.go"), "package main")
	writeTestFile(t, filepath.Join(pkgDir, "schema.go"), "package db")
	writeTestFile(t, filepath.Join(otherDir, "index.html"), "<html>")

	watch := []string{appDir, pkgDir}

	h1, err := HashBuildContext(dir, "", watch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Changing a watched file changes the hash
	writeTestFile(t, filepath.Join(appDir, "main.go"), "package main // v2")
	h2, _ := HashBuildContext(dir, "", watch)
	if h1 == h2 {
		t.Error("expected hash to change when watched file changes")
	}

	// Changing an unwatched file does NOT change the hash
	h3, _ := HashBuildContext(dir, "", watch)
	writeTestFile(t, filepath.Join(otherDir, "index.html"), "<html>changed</html>")
	h4, _ := HashBuildContext(dir, "", watch)
	if h3 != h4 {
		t.Error("expected hash to NOT change when unwatched file changes")
	}
}

func TestHashBuildContext_DockerfileScopesDir(t *testing.T) {
	dir := t.TempDir()
	apiDir := filepath.Join(dir, "apps", "api")
	webDir := filepath.Join(dir, "apps", "web")

	if err := os.MkdirAll(apiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(webDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(apiDir, "Dockerfile"), "FROM node")
	writeTestFile(t, filepath.Join(apiDir, "index.ts"), "console.log('api')")
	writeTestFile(t, filepath.Join(webDir, "index.html"), "<html>")

	dockerfile := filepath.Join(apiDir, "Dockerfile")

	h1, err := HashBuildContext(dir, dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Changing a file in the Dockerfile's directory changes the hash
	writeTestFile(t, filepath.Join(apiDir, "index.ts"), "console.log('api v2')")
	h2, _ := HashBuildContext(dir, dockerfile, nil)
	if h1 == h2 {
		t.Error("expected hash to change when Dockerfile-scoped file changes")
	}

	// Changing a file outside the Dockerfile's directory does NOT change the hash
	h3, _ := HashBuildContext(dir, dockerfile, nil)
	writeTestFile(t, filepath.Join(webDir, "index.html"), "<html>v2</html>")
	h4, _ := HashBuildContext(dir, dockerfile, nil)
	if h3 != h4 {
		t.Error("expected hash to NOT change when file outside Dockerfile dir changes")
	}
}

func TestHashBuildContext_Dockerignore(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".dockerignore"), "node_modules\n*.log\n")
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main")
	writeTestFile(t, filepath.Join(dir, "app.log"), "some log")

	nmDir := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nmDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(nmDir, "dep.js"), "module.exports = {}")

	h1, err := HashBuildContext(dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Changing an ignored file should NOT change the hash
	writeTestFile(t, filepath.Join(dir, "app.log"), "new log content")
	h2, _ := HashBuildContext(dir, "", nil)
	if h1 != h2 {
		t.Error("expected hash to NOT change when dockerignored file changes")
	}

	// Changing a non-ignored file should change the hash
	writeTestFile(t, filepath.Join(dir, "main.go"), "package main // v2")
	h3, _ := HashBuildContext(dir, "", nil)
	if h1 == h3 {
		t.Error("expected hash to change when non-ignored file changes")
	}
}

func TestSave_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := New()
	if err := s.Save(path); err != nil {
		t.Fatalf("save error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}
