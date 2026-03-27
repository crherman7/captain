package state

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	// Load .dockerignore patterns
	ignorePatterns := loadDockerignore(contextDir)

	// Determine which directories to hash
	var scanDirs []string
	switch {
	case len(watchPaths) > 0:
		scanDirs = watchPaths
	case dockerfile != "":
		scanDirs = []string{filepath.Dir(dockerfile)}
	default:
		scanDirs = []string{contextDir}
	}

	// Collect files from all scan dirs
	var files []string
	for _, dir := range scanDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", dir, err)
		}
		if !info.IsDir() {
			// Single file
			if !isIgnored(dir, contextDir, ignorePatterns) {
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
			if isIgnored(path, contextDir, ignorePatterns) {
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

	// Always include the Dockerfile if not already hashed
	if dockerfile != "" {
		absDockerfile, _ := filepath.Abs(dockerfile)
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
			data, err := os.ReadFile(dockerfile)
			if err != nil {
				return "", fmt.Errorf("reading dockerfile %s: %w", dockerfile, err)
			}
			h.Write(data)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// loadDockerignore reads .dockerignore from the given directory and returns patterns.
func loadDockerignore(contextDir string) []string {
	path := filepath.Join(contextDir, ".dockerignore")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// isIgnored checks if a file path matches any .dockerignore pattern.
func isIgnored(path, contextDir string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}

	rel, err := filepath.Rel(contextDir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)

	ignored := false
	for _, pattern := range patterns {
		negate := false
		if strings.HasPrefix(pattern, "!") {
			negate = true
			pattern = pattern[1:]
		}

		if matchPattern(rel, pattern) {
			ignored = !negate
		}
	}
	return ignored
}

// matchPattern matches a relative path against a dockerignore pattern.
// Supports ** (any number of path segments), * (single segment glob), and plain names.
func matchPattern(rel, pattern string) bool {
	pattern = filepath.ToSlash(pattern)

	// Handle ** prefix: match pattern suffix against any path segment
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		// Try matching suffix against the full rel and every sub-path
		parts := strings.Split(rel, "/")
		for i := range parts {
			sub := strings.Join(parts[i:], "/")
			if matchSimple(sub, suffix) {
				return true
			}
		}
		return false
	}

	// Handle ** anywhere in pattern by trying all segment combinations
	if strings.Contains(pattern, "**") {
		// Replace ** with match-all and try each segment start
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")
			if prefix == "" {
				return matchSimple(rel, suffix)
			}
			if strings.HasPrefix(rel, prefix+"/") || matchSimple(rel, prefix) {
				rest := strings.TrimPrefix(rel, prefix+"/")
				if suffix == "" {
					return true
				}
				return matchSimple(rest, suffix)
			}
		}
		return false
	}

	return matchSimple(rel, pattern)
}

// matchSimple matches without ** support — handles plain names, directory prefixes, and * globs.
func matchSimple(rel, pattern string) bool {
	// Exact match
	if rel == pattern {
		return true
	}

	// Directory prefix match (pattern "node_modules" matches "node_modules/foo/bar.js")
	if strings.HasPrefix(rel, pattern+"/") {
		return true
	}

	// Check if any path component matches the pattern (e.g., "node_modules" matches "apps/api/node_modules/x")
	if !strings.Contains(pattern, "/") {
		for _, part := range strings.Split(rel, "/") {
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}
	}

	// filepath.Match against full relative path
	if matched, _ := filepath.Match(pattern, rel); matched {
		return true
	}

	return false
}
