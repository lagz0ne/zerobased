package run

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lagz0ne/zerobased/internal/caddy"
	"github.com/lagz0ne/zerobased/internal/classifier"
	"github.com/lagz0ne/zerobased/internal/daemon"
	"github.com/lagz0ne/zerobased/internal/docker"
	"github.com/lagz0ne/zerobased/internal/env"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// Options for the run command.
type Options struct {
	Name       string   // route name (defaults to cwd basename)
	Args       []string // command + args
	Port       int      // dev server port (0 = auto-detect)
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

	port := opts.Port
	if port == 0 {
		port = detectPort(opts.Args)
	}
	if port == 0 {
		port = 3000
	}

	hostname := fmt.Sprintf("%s.localhost", name)
	routeID := fmt.Sprintf("zb-run-%s", name)
	upstream := fmt.Sprintf("localhost:%d", port)

	// Resolve project services and build env vars
	zbEnv := resolveProjectEnv(name, opts.DockerHost, opts.EnvPrefix)

	cmd := exec.Command(opts.Args[0], opts.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), zbEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if len(zbEnv) > 0 {
		log.Printf("injected %d env vars for project %q", len(zbEnv), name)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", opts.Args[0], err)
	}

	if err := registerRoute(routeID, hostname, upstream); err != nil {
		log.Printf("warning: failed to register route: %v", err)
		log.Printf("app will still be available at localhost:%d", port)
	} else {
		log.Printf("→ http://%s", hostname)
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

	deregisterRoute(routeID)
	return nil
}

// resolveProjectEnv queries Docker for compose services matching the project name
// and returns ZB_<SERVICE>_<PORT>=<conn_string> env vars.
func resolveProjectEnv(project, dockerHost, prefix string) []string {
	dc, err := docker.NewWithHost(dockerHost)
	if err != nil {
		return nil
	}
	defer dc.Close()

	containers, err := dc.ListRunning(context.Background())
	if err != nil {
		return nil
	}

	baseDir := daemon.DefaultBaseDir()
	var endpoints []env.ServiceEndpoint
	for _, c := range containers {
		if c.Project != project {
			continue
		}
		for _, pb := range c.Ports {
			if pb.Proto != "tcp" {
				continue
			}
			method := classifier.ClassifyFromLabels(pb.ContainerPort, c.Labels)
			if method == classifier.Internal {
				continue
			}
			endpoints = append(endpoints, env.ForPort(baseDir, c.Project, c.Service, pb.ContainerPort, method))
		}
	}

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
		strings.NewReader(string(body)),
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
	if resp, err := httpClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

func detectPort(args []string) int {
	cmdStr := strings.Join(args, " ")
	switch {
	case strings.Contains(cmdStr, "vite"):
		return 5173
	case strings.Contains(cmdStr, "next"):
		return 3000
	case strings.Contains(cmdStr, "nuxt"):
		return 3000
	default:
		return 0
	}
}
