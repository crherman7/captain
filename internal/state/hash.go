package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/patternmatcher"
)

// ComputeHash produces a SHA-256 hash from resolved Helm values and an optional build context hash.
func ComputeHash(values map[string]interface{}, buildHash string) (string, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("marshaling values for hash: %w", err)
	}

	h := sha256.New()
	h.Write(data)
	h.Write([]byte(buildHash))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// HashBuildContext produces a deterministic hash of build inputs.
//
// Scoping rules (in order of precedence):
//  1. If watchPaths is non-empty, only those paths are hashed.
//  2. If dockerfile is set, the Dockerfile's parent directory is hashed.
//  3. Otherwise, the entire contextDir is hashed.
//
// A .dockerignore file in contextDir is respected in all cases.
// The Dockerfile itself is always included in the hash.
func HashBuildContext(contextDir, dockerfile string, watchPaths []string) (string, error) {
	h := sha256.New()

	ignorePatterns, err := loadDockerignore(contextDir)
	if err != nil {
		return "", err
	}

	var scanDirs []string
	switch {
	case len(watchPaths) > 0:
		scanDirs = watchPaths
	case dockerfile != "":
		dir := filepath.Dir(dockerfile)
		if dir == "." {
			dir = contextDir
		}
		scanDirs = []string{dir}
	default:
		scanDirs = []string{contextDir}
	}

	var files []string
	for _, dir := range scanDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", dir, err)
		}
		if !info.IsDir() {
			ignored, err := isIgnored(ignorePatterns, dir, contextDir)
			if err != nil {
				return "", fmt.Errorf("checking ignore for %s: %w", dir, err)
			}
			if !ignored {
				files = append(files, dir)
			}
			continue
		}
		err = filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			ignored, err := isIgnored(ignorePatterns, path, contextDir)
			if err != nil {
				return fmt.Errorf("checking ignore for %s: %w", path, err)
			}
			if ignored {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walking %s: %w", dir, err)
		}
	}

	sort.Strings(files)

	for _, path := range files {
		h.Write([]byte(path))

		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return "", fmt.Errorf("hashing %s: %w", path, err)
		}
		_ = f.Close()
	}

	// Always include the Dockerfile if not already hashed.
	// Resolve a bare filename (no directory component) relative to contextDir.
	if dockerfile != "" {
		resolvedDockerfile := dockerfile
		if filepath.Dir(dockerfile) == "." {
			resolvedDockerfile = filepath.Join(contextDir, dockerfile)
		}
		absDockerfile, _ := filepath.Abs(resolvedDockerfile)
		alreadyHashed := false
		for _, f := range files {
			absF, _ := filepath.Abs(f)
			if absF == absDockerfile {
				alreadyHashed = true
				break
			}
		}
		if !alreadyHashed {
			h.Write([]byte("Dockerfile"))
			data, err := os.ReadFile(resolvedDockerfile)
			if err != nil {
				return "", fmt.Errorf("reading dockerfile %s: %w", resolvedDockerfile, err)
			}
			h.Write(data)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// loadDockerignore reads .dockerignore from contextDir and returns its patterns.
// Returns nil (no error) if the file does not exist.
func loadDockerignore(contextDir string) ([]string, error) {
	path := filepath.Join(contextDir, ".dockerignore")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading .dockerignore: %w", err)
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	return patterns, nil
}

// isIgnored reports whether path should be excluded according to the .dockerignore patterns.
func isIgnored(patterns []string, path, contextDir string) (bool, error) {
	if len(patterns) == 0 {
		return false, nil
	}
	rel, err := filepath.Rel(contextDir, path)
	if err != nil {
		return false, err
	}
	return patternmatcher.MatchesOrParentMatches(filepath.ToSlash(rel), patterns)
}
