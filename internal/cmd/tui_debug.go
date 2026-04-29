package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	tuiDebugOnce sync.Once
	tuiDebugMu   sync.Mutex
	tuiDebugFile *os.File
)

func tuiDebugEnabled() bool {
	return os.Getenv("CAPTAIN_TUI_DEBUG") != ""
}

func tuiDebugPath() string {
	return filepath.Join(os.TempDir(), "captain-tui-debug.log")
}

func tuiDebugf(format string, args ...interface{}) {
	if !tuiDebugEnabled() {
		return
	}

	tuiDebugOnce.Do(func() {
		f, err := os.OpenFile(tuiDebugPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err == nil {
			tuiDebugFile = f
		}
	})

	if tuiDebugFile == nil {
		return
	}

	tuiDebugMu.Lock()
	defer tuiDebugMu.Unlock()

	ts := time.Now().Format("15:04:05.000")
	_, _ = fmt.Fprintf(tuiDebugFile, "%s %s\n", ts, fmt.Sprintf(format, args...))
}
