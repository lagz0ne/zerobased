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
			printHelp()
			return
		case "start":
			runStart()
			return
		case "up":
			runUp()
			return
		}
	}

	printHelp()
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

func printHelp() {
	fmt.Printf(`zerobased %s

Explicit local dev stack orchestration.

Usage:
  zerobased start      Start the machine-local control plane and route runtime.
  zerobased up         Run the current project from explicit zerobased.yaml.
  zerobased version    Print the zerobased version.
  zerobased help       Show this help.

zerobased.yaml v1:
  version: 1
  name: my-project
  host: my-project.localhost
  compose:
    files:
      - compose.yaml
    services:
      - postgres
    ownership: owned
  processes:
    web:
      command: ["go", "run", "./cmd/web"]
      readiness:
        type: file
        path: .zerobased/web.ready
  routes:
    - path: /
      process: web
      port: 3000

How it works:
  start runs once per machine and owns local routing on 127.0.0.1:80.
  up starts only the Compose files and services declared in zerobased.yaml,
  starts declared local processes, waits for readiness, publishes routes,
  and stays foreground until Ctrl-C.

Rules:
  zerobased.yaml must start with version: 1.
  Compose files are not discovered; list them under compose.files.
  Every process must declare file readiness with type and path.
  Owned Compose rejects container_name, host ports, host networking,
  host namespace sharing, provider-managed services, external resources,
  and custom resource names before containers start.
  With the default Docker Caddy router, local web processes must listen on
  0.0.0.0, not only 127.0.0.1.
`, version)
}
