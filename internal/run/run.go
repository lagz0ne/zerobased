package run

import (
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
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// Run wraps a dev server process and registers it as an HTTP route with Caddy.
func Run(name string, args []string, port int) error {
	if name == "" {
		dir, _ := os.Getwd()
		name = filepath.Base(dir)
	}

	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	if port == 0 {
		port = detectPort(args)
	}
	if port == 0 {
		port = 3000
	}

	hostname := fmt.Sprintf("%s.localhost", name)
	routeID := fmt.Sprintf("zb-run-%s", name)
	upstream := fmt.Sprintf("localhost:%d", port)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", args[0], err)
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
