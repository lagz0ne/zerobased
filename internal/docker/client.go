package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// ContainerInfo holds the extracted data zerobased needs from a container.
type ContainerInfo struct {
	ID        string
	Name      string
	Project   string // com.docker.compose.project
	Service   string // com.docker.compose.service
	IP        string // container IP on bridge network
	Ports     []PortBinding
	Labels    map[string]string
}

// PortBinding represents one exposed container port.
type PortBinding struct {
	ContainerPort uint16
	Proto         string // "tcp" or "udp"
}

// Client wraps the Docker SDK client with zerobased-specific methods.
type Client struct {
	cli *client.Client
}

// New creates a Docker client connected to the local socket.
func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close closes the underlying Docker client.
func (c *Client) Close() error {
	return c.cli.Close()
}

// Inner returns the underlying Docker SDK client for direct use.
func (c *Client) Inner() *client.Client {
	return c.cli
}

// Events returns a channel of container start/stop events.
func (c *Client) Events(ctx context.Context) (<-chan events.Message, <-chan error) {
	f := filters.NewArgs()
	f.Add("type", string(events.ContainerEventType))
	f.Add("event", "start")
	f.Add("event", "die")
	return c.cli.Events(ctx, events.ListOptions{Filters: f})
}

// Inspect extracts the ContainerInfo from a running container.
func (c *Client) Inspect(ctx context.Context, containerID string) (*ContainerInfo, error) {
	cj, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", containerID, err)
	}

	info := &ContainerInfo{
		ID:      cj.ID,
		Name:    strings.TrimPrefix(cj.Name, "/"),
		Project: cj.Config.Labels["com.docker.compose.project"],
		Service: cj.Config.Labels["com.docker.compose.service"],
		Labels:  cj.Config.Labels,
	}

	// Get container IP from network settings
	if cj.NetworkSettings != nil {
		for _, net := range cj.NetworkSettings.Networks {
			if net.IPAddress != "" {
				info.IP = net.IPAddress
				break
			}
		}
	}

	// Extract exposed ports from container config
	if cj.Config != nil {
		for portSpec := range cj.Config.ExposedPorts {
			port, _ := strconv.ParseUint(portSpec.Port(), 10, 16)
			if port > 0 {
				info.Ports = append(info.Ports, PortBinding{
					ContainerPort: uint16(port),
					Proto:         portSpec.Proto(),
				})
			}
		}
	}

	return info, nil
}

// ListRunning returns ContainerInfo for all currently running containers.
func (c *Client) ListRunning(ctx context.Context) ([]*ContainerInfo, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	var infos []*ContainerInfo
	for _, ctr := range containers {
		info, err := c.Inspect(ctx, ctr.ID)
		if err != nil {
			continue // skip containers that disappear between list and inspect
		}
		if info.Project == "" {
			continue // skip non-compose containers
		}
		infos = append(infos, info)
	}
	return infos, nil
}
