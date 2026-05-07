package hash

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing test file %s: %v", path, err)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	values := map[string]interface{}{"a": "1", "b": "2"}

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

	for _, d := range []string{appDir, pkgDir, otherDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	writeTestFile(t, filepath.Join(appDir, "main.go"), "package main")
	writeTestFile(t, filepath.Join(pkgDir, "schema.go"), "package db")
	writeTestFile(t, filepath.Join(otherDir, "index.html"), "<html>")

	watch := []string{appDir, pkgDir}

	h1, err := HashBuildContext(dir, "", watch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	writeTestFile(t, filepath.Join(appDir, "main.go"), "package main // v2")
	h2, _ := HashBuildContext(dir, "", watch)
	if h1 == h2 {
		t.Error("expected hash to change when watched file changes")
	}

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

	for _, d := range []string{apiDir, webDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	writeTestFile(t, filepath.Join(apiDir, "Dockerfile"), "FROM node")
	writeTestFile(t, filepath.Join(apiDir, "index.ts"), "console.log('api')")
	writeTestFile(t, filepath.Join(webDir, "index.html"), "<html>")

	dockerfile := filepath.Join(apiDir, "Dockerfile")

	h1, err := HashBuildContext(dir, dockerfile, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	writeTestFile(t, filepath.Join(apiDir, "index.ts"), "console.log('api v2')")
	h2, _ := HashBuildContext(dir, dockerfile, nil)
	if h1 == h2 {
		t.Error("expected hash to change when Dockerfile-scoped file changes")
	}

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

	writeTestFile(t, filepath.Join(dir, "app.log"), "new log content")
	h2, _ := HashBuildContext(dir, "", nil)
	if h1 != h2 {
		t.Error("expected hash to NOT change when dockerignored file changes")
	}

	writeTestFile(t, filepath.Join(dir, "main.go"), "package main // v2")
	h3, _ := HashBuildContext(dir, "", nil)
	if h1 == h3 {
		t.Error("expected hash to change when non-ignored file changes")
	}
}
