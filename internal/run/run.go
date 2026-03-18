package run

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// Matches lines like "Local: http://localhost:5173/" or "ready in 123ms → http://localhost:3000"
var portPattern = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d+)`)

// Options for the run command.
type Options struct {
	Name       string   // route name (defaults to cwd basename)
	Args       []string // command + args
	Port       int      // dev server port (0 = auto-detect from stdout)
	DockerHost string   // custom docker host
	EnvPrefix  string   // env var prefix (default "ZB", empty "" for no prefix)
}

// Run wraps a dev server process, injects ZB_* env vars for project services,
// registers an HTTP route with Caddy, and cleans up on exit.
func Run(opts Options) error {
	name := opts.Name
	if name == "" {
		dir, _ := os.Getwd()
		name = filepath.Base(dir)
	}

	if len(opts.Args) == 0 {
		return fmt.Errorf("no command specified")
	}

	hostname := fmt.Sprintf("%s.localhost", name)
	routeID := fmt.Sprintf("zb-run-%s", name)

	// Resolve project services and build env vars
	zbEnv := resolveProjectEnv(name, opts.DockerHost, opts.EnvPrefix)

	cmd := exec.Command(opts.Args[0], opts.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if len(zbEnv) > 0 {
		cmd.Env = append(os.Environ(), zbEnv...)
		log.Printf("injected %d env vars for project %q", len(zbEnv), name)
	}

	if opts.Port > 0 {
		// Explicit port — register route immediately, pass through stdout/stderr
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", opts.Args[0], err)
		}

		upstream := fmt.Sprintf("localhost:%d", opts.Port)
		if err := registerRoute(routeID, hostname, upstream); err != nil {
			log.Printf("warning: failed to register route: %v", err)
		} else {
			log.Printf("→ http://%s (port %d)", hostname, opts.Port)
		}
	} else {
		// Auto-detect port from stdout+stderr — tee output while scanning for URL
		pr, pw := io.Pipe()
		cmd.Stdout = io.MultiWriter(os.Stdout, pw)
		cmd.Stderr = io.MultiWriter(os.Stderr, pw)

		if err := cmd.Start(); err != nil {
			pw.Close()
			return fmt.Errorf("start %s: %w", opts.Args[0], err)
		}

		// Scan stdout in background for port, with timeout
		portCh := make(chan int, 1)
		go func() {
			defer pw.Close()
			scanner := bufio.NewScanner(pr)
			for scanner.Scan() {
				line := scanner.Text()
				if m := portPattern.FindStringSubmatch(line); m != nil {
					if p, err := strconv.Atoi(m[1]); err == nil {
						portCh <- p
						// Keep draining to avoid blocking the pipe
						io.Copy(io.Discard, pr)
						return
					}
				}
			}
			portCh <- 0 // no port found
		}()

		select {
		case port := <-portCh:
			if port > 0 {
				upstream := fmt.Sprintf("localhost:%d", port)
				if err := registerRoute(routeID, hostname, upstream); err != nil {
					log.Printf("warning: failed to register route: %v", err)
				} else {
					log.Printf("→ http://%s (detected port %d)", hostname, port)
				}
			} else {
				log.Printf("warning: could not detect port — use -p to specify")
			}
		case <-time.After(30 * time.Second):
			log.Printf("warning: port detection timed out after 30s — use -p to specify")
		}
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case sig := <-sigs:
		syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		<-done
	case <-done:
	}

	signal.Stop(sigs)
	deregisterRoute(routeID)
	return nil
}

// resolveProjectEnv queries Docker for compose services matching the project name.
func resolveProjectEnv(project, dockerHost, prefix string) []string {
	dc, err := docker.NewWithHost(dockerHost)
	if err != nil {
		log.Printf("warning: docker unavailable for env injection: %v", err)
		return nil
	}
	defer dc.Close()

	containers, err := dc.ListRunning(context.Background())
	if err != nil {
		log.Printf("warning: failed to list containers for env injection: %v", err)
		return nil
	}

	baseDir := daemon.DefaultBaseDir()
	endpoints := env.EndpointsFromContainers(baseDir, containers, project)
	return env.AsEnvVars(prefix, endpoints)
}

func registerRoute(routeID, hostname, upstream string) error {
	route := map[string]any{
		"@id": routeID,
		"match": []map[string]any{
			{"host": []string{hostname}},
		},
		"handle": []map[string]any{
			{
				"handler": "reverse_proxy",
				"upstreams": []map[string]string{
					{"dial": upstream},
				},
			},
		},
	}

	body, err := json.Marshal(route)
	if err != nil {
		return err
	}

	resp, err := httpClient.Post(
		caddy.AdminAddr+"/config/apps/http/servers/zerobased/routes",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("caddy %d", resp.StatusCode)
	}
	return nil
}

func deregisterRoute(routeID string) {
	req, err := http.NewRequest("DELETE", caddy.AdminAddr+"/id/"+routeID, nil)
	if err != nil {
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("warning: failed to deregister route %s: %v", routeID, err)
		return
	}
	resp.Body.Close()
}
