package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	containerName = "zerobased-caddy"
	caddyImage    = "caddy:2-alpine"
	adminAddr     = "http://localhost:2019"
)

// Manager manages the Caddy container and its admin API for route registration.
type Manager struct {
	docker *client.Client
	http   *http.Client
}

// New creates a Caddy manager using the provided Docker client.
func New(docker *client.Client) *Manager {
	return &Manager{
		docker: docker,
		http:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Start ensures the Caddy container is running. Creates it if it doesn't exist.
func (m *Manager) Start(ctx context.Context) error {
	// Check if already running
	cj, err := m.docker.ContainerInspect(ctx, containerName)
	if err == nil && cj.State.Running {
		return nil
	}

	// Remove stopped container if exists
	if err == nil {
		m.docker.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})
	}

	// Pull image
	reader, err := m.docker.ImagePull(ctx, caddyImage, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull caddy image: %w", err)
	}
	io.Copy(io.Discard, reader)
	reader.Close()

	// Caddyfile that enables the admin API and listens on :80 with auto_https off
	caddyfile := `{
	admin 0.0.0.0:2019
	auto_https off
}
`

	resp, err := m.docker.ContainerCreate(ctx,
		&container.Config{
			Image: caddyImage,
			Cmd:   []string{"caddy", "run", "--adapter", "caddyfile", "--config", "/dev/stdin"},
			ExposedPorts: nat.PortSet{
				nat.Port("80/tcp"):   struct{}{},
				nat.Port("2019/tcp"): struct{}{},
			},
			OpenStdin: true,
		},
		&container.HostConfig{
			NetworkMode: "host",
			RestartPolicy: container.RestartPolicy{
				Name: container.RestartPolicyAlways,
			},
		},
		&network.NetworkingConfig{},
		nil,
		containerName,
	)
	if err != nil {
		return fmt.Errorf("create caddy container: %w", err)
	}

	// Attach to write Caddyfile via stdin
	attach, err := m.docker.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return fmt.Errorf("attach caddy: %w", err)
	}

	if err := m.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		attach.Close()
		return fmt.Errorf("start caddy: %w", err)
	}

	// Write Caddyfile through stdin and close
	attach.Conn.Write([]byte(caddyfile))
	attach.CloseWrite()
	attach.Close()

	// Wait for admin API to be ready
	for i := 0; i < 30; i++ {
		time.Sleep(200 * time.Millisecond)
		r, err := m.http.Get(adminAddr + "/config/")
		if err == nil {
			r.Body.Close()
			return nil
		}
	}

	return fmt.Errorf("caddy admin API not ready after 6s")
}

// Stop removes the Caddy container.
func (m *Manager) Stop(ctx context.Context) error {
	return m.docker.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true})
}

// AddHTTPRoute registers a hostname → upstream reverse proxy route via the Caddy admin API.
// hostname: e.g., "nats-80.acountee.localhost"
// upstream: e.g., "172.17.0.3:80"
func (m *Manager) AddHTTPRoute(routeID, hostname, upstream string) error {
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

	return m.postRoute(route)
}

// AddTCPRoute registers a listen port → upstream TCP proxy.
// listenPort: the deterministic hashed port
// upstream: e.g., "172.17.0.3:4222"
func (m *Manager) AddTCPRoute(routeID string, listenPort uint16, upstream string) error {
	// For TCP proxying, we use Caddy's reverse_proxy with a specific listen address.
	// Since Caddy L4 isn't in the base image, we use a simple HTTP reverse proxy on a specific port.
	route := map[string]any{
		"@id": routeID,
		"match": []map[string]any{
			{"host": []string{fmt.Sprintf(":%d", listenPort)}},
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

	return m.postRoute(route)
}

// RemoveRoute removes a route by its ID.
func (m *Manager) RemoveRoute(routeID string) error {
	req, err := http.NewRequest("DELETE", adminAddr+"/id/"+routeID, nil)
	if err != nil {
		return err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("delete route %s: %w", routeID, err)
	}
	resp.Body.Close()
	return nil
}

func (m *Manager) postRoute(route map[string]any) error {
	body, err := json.Marshal(route)
	if err != nil {
		return err
	}

	resp, err := m.http.Post(
		adminAddr+"/config/apps/http/servers/zerobased/routes",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("add route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy API %d: %s", resp.StatusCode, b)
	}
	return nil
}

// EnsureHTTPServer creates the base HTTP server config if it doesn't exist.
func (m *Manager) EnsureHTTPServer() error {
	server := map[string]any{
		"listen": []string{":80"},
		"routes": []any{},
	}

	body, err := json.Marshal(map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"zerobased": server,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", adminAddr+"/load", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy load %d: %s", resp.StatusCode, b)
	}
	return nil
}
