package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintUsageHighlightsPreferredYAMLRoutefile(t *testing.T) {
	out := captureStdout(t, func() {
		printUsage(nil)
	})

	if !strings.Contains(out, "only supported routefile format") {
		t.Fatalf("top-level help should make YAML-only support explicit, got:\n%s", out)
	}

	if strings.Contains(out, "Legacy zerobased.routes") {
		t.Fatalf("top-level help should not mention the removed legacy routefile, got:\n%s", out)
	}

	if !strings.Contains(out, "up [--profile name] [--set k=v]") {
		t.Fatalf("top-level help should list the up command, got:\n%s", out)
	}
}

func TestPrintUsageShowsVersionBeforeUsage(t *testing.T) {
	orig := version
	version = "test-version"
	defer func() {
		version = orig
	}()

	out := captureStdout(t, func() {
		printUsage(nil)
	})

	versionIdx := strings.Index(out, "Version: test-version")
	usageIdx := strings.Index(out, "Usage:")
	if versionIdx == -1 {
		t.Fatalf("top-level help should print the current version, got:\n%s", out)
	}
	if usageIdx == -1 || versionIdx > usageIdx {
		t.Fatalf("version should appear before usage, got:\n%s", out)
	}
}

func TestPrintCommandHelpRunHighlightsYAMLRoutefile(t *testing.T) {
	out := captureStdout(t, func() {
		printCommandHelp("run")
	})

	if !strings.Contains(out, "zerobased.routes.yaml") {
		t.Fatalf("run help should mention YAML routefiles, got:\n%s", out)
	}

	if !strings.Contains(out, "only supported") {
		t.Fatalf("run help should mark YAML routefiles as the only supported format, got:\n%s", out)
	}

	if strings.Contains(out, "Legacy zerobased.routes") {
		t.Fatalf("run help should not mention the removed legacy routefile, got:\n%s", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = orig
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	return <-done
}
