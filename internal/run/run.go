package run

import (
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
)

const caddyAdmin = "http://localhost:2019"

// Run wraps a dev server process and registers it as an HTTP route with Caddy.
// name: route name (defaults to current directory name)
// args: the command + args to run
// port: the local port the dev server listens on (auto-detected from common frameworks if 0)
func Run(name string, args []string, port int) error {
	if name == "" {
		dir, _ := os.Getwd()
		name = filepath.Base(dir)
	}

	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Default port for common frameworks
	if port == 0 {
		port = detectPort(args)
	}
	if port == 0 {
		port = 3000 // fallback
	}

	hostname := fmt.Sprintf("%s.localhost", name)
	routeID := fmt.Sprintf("zb-run-%s", name)
	upstream := fmt.Sprintf("localhost:%d", port)

	// Start the child process
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", args[0], err)
	}

	// Register route with Caddy
	if err := registerRoute(routeID, hostname, upstream); err != nil {
		log.Printf("warning: failed to register route: %v", err)
		log.Printf("app will still be available at localhost:%d", port)
	} else {
		log.Printf("→ http://%s", hostname)
	}

	// Trap signals for cleanup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case sig := <-sigs:
		// Forward signal to process group
		syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
		<-done
	case <-done:
	}

	// Cleanup route
	deregisterRoute(routeID)
	return nil
}

func registerRoute(routeID, hostname, upstream string) error {
	body := fmt.Sprintf(`{"@id":"%s","match":[{"host":["%s"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"%s"}]}]}`,
		routeID, hostname, upstream)

	resp, err := http.Post(
		caddyAdmin+"/config/apps/http/servers/zerobased/routes",
		"application/json",
		strings.NewReader(body),
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
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("DELETE", caddyAdmin+"/id/"+routeID, nil)
	if resp, err := client.Do(req); err == nil {
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
