package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lagz0ne/zerobased/internal/controlplane"
	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/up"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println(version)
			return
		case "help", "--help", "-h":
			printRewriteNotice()
			return
		case "start":
			runStart()
			return
		case "up":
			runUp()
			return
		}
	}

	printRewriteNotice()
	os.Exit(1)
}

func runStart() {
	home, err := resolveHome()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zerobased start: %v\n", err)
		os.Exit(1)
	}

	result, err := daemon.Start(daemon.ConfigFromEnv(home))
	if err != nil {
		fmt.Fprintf(os.Stderr, "zerobased start: %v\n", err)
		os.Exit(1)
	}
	defer result.Close()

	fmt.Printf("zerobased daemon ready\nhome: %s\nsocket: %s\nhealth: %s\n", home, result.SocketPath, result.HealthPath)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}

func runUp() {
	home, err := resolveHome()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zerobased up: %v\n", err)
		os.Exit(1)
	}
	projectDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zerobased up: get cwd: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = up.Run(ctx, up.Options{
		ProjectDir: projectDir,
		ControlPlane: controlplane.Client{
			SocketPath: filepath.Join(home, "daemon.sock"),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "zerobased up: %v\n", err)
		os.Exit(1)
	}
}

func resolveHome() (string, error) {
	home := os.Getenv("ZEROBASED_HOME")
	if home != "" {
		return home, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(userHome, ".zerobased"), nil
}

func printRewriteNotice() {
	fmt.Printf(`zerobased %s

Rewrite in progress.

The legacy Docker autodiscovery router has been removed from this working tree.
The current explicit local control-plane contract is described in:
  docs/superpowers/specs/2026-05-13-zerobased-control-plane-rfc.md

First-class product boundaries for the rewrite:
  zerobased start
  zerobased up

zerobased.yaml v1 starts with:
  version: 1
  name: my-project
  compose:
    files:
      - compose.yaml
    services:
      - postgres
    ownership: owned
  processes: ...
  routes: ...

Compose is explicit: zerobased only loads files listed in zerobased.yaml.
Owned Compose services get a zerobased-generated project name and dev.zerobased.* labels.
Global Compose escape hatches like container_name, host ports, host networking,
external resources, and custom resource names fail before containers start.

Run start in one terminal, then run up from a project with zerobased.yaml.
up stays foreground until stopped.

Implementation must follow traced-TDD failure ownership before behavior lands.
`, version)
}
