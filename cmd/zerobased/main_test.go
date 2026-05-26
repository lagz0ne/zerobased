package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintRewriteNotice(t *testing.T) {
	out := captureStdout(t, printRewriteNotice)

	for _, want := range []string{
		"Rewrite in progress.",
		"zerobased start",
		"zerobased up",
		"compose:",
		"files:",
		"traced-TDD failure ownership",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("notice missing %q\n%s", want, out)
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
