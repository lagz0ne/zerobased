package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintHelpTextIsUserFacing(t *testing.T) {
	oldVersion := version
	version = "v.test"
	t.Cleanup(func() {
		version = oldVersion
	})

	out := captureStdout(t, printHelp)

	for _, want := range []string{
		"zerobased v.test\n\n",
		"Explicit local dev stack orchestration.",
		"Usage:",
		"zerobased start",
		"zerobased up",
		"zerobased.yaml v1:",
		"version: 1",
		"host: my-project.localhost",
		"compose:",
		"files:",
		"routes:",
		"stays foreground until Ctrl-C",
		"Every process must declare file readiness with type and path.",
		"host namespace sharing",
		"provider-managed services",
		"0.0.0.0, not only 127.0.0.1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q\n%s", want, out)
		}
	}

	for _, unwanted := range []string{
		"Rewrite",
		"legacy",
		"docs/",
		"superpowers",
		"RFC",
		"TDD",
		"traced",
		"Implementation must",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("help contains internal term %q\n%s", unwanted, out)
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()
	w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
