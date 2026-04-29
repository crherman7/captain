package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	restclient "k8s.io/client-go/rest"
)

type tuiWarningCollector struct {
	mu       sync.Mutex
	order    []string
	warnings map[string]struct{}
}

func newTUIWarningCollector() *tuiWarningCollector {
	return &tuiWarningCollector{
		warnings: make(map[string]struct{}),
	}
}

func (c *tuiWarningCollector) HandleWarningHeaderWithContext(_ context.Context, code int, _ string, message string) {
	if code != 299 || message == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.warnings[message]; exists {
		return
	}
	c.warnings[message] = struct{}{}
	c.order = append(c.order, message)
}

func (c *tuiWarningCollector) FlushTo(w io.Writer) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.order) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	for _, warning := range c.order {
		_, _ = fmt.Fprintf(w, "  Warning: %s\n", warning)
	}
}

func (c *tuiWarningCollector) Write(p []byte) (int, error) {
	for _, line := range bytes.Split(p, []byte{'\n'}) {
		msg := string(bytes.TrimSpace(line))
		if msg == "" {
			continue
		}
		c.HandleWarningHeaderWithContext(context.Background(), 299, "", msg)
	}
	return len(p), nil
}

func installTUIWarningHandler() *tuiWarningCollector {
	collector := newTUIWarningCollector()
	restclient.SetDefaultWarningHandlerWithContext(collector)
	return collector
}

func restoreDefaultWarningHandler() {
	restclient.SetDefaultWarningHandlerWithContext(restclient.WarningLogger{})
}

type tuiStdLogCapture struct {
	collector *tuiWarningCollector
	prevOut   io.Writer
	prevFlags int
	prevPref  string
}

func installTUIStdLogCapture() *tuiStdLogCapture {
	c := &tuiStdLogCapture{
		collector: newTUIWarningCollector(),
		prevOut:   log.Writer(),
		prevFlags: log.Flags(),
		prevPref:  log.Prefix(),
	}
	log.SetOutput(c.collector)
	log.SetFlags(0)
	log.SetPrefix("")
	return c
}

func (c *tuiStdLogCapture) restore() {
	log.SetOutput(c.prevOut)
	log.SetFlags(c.prevFlags)
	log.SetPrefix(c.prevPref)
}

func (c *tuiStdLogCapture) FlushTo(w io.Writer) {
	if c == nil || c.collector == nil {
		return
	}
	c.collector.FlushTo(w)
}
