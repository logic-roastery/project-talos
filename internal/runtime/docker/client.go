package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Client struct {
	cli     *client.Client
	network string
	logger  *slog.Logger
}

func NewClient(dockerHost, network string, logger *slog.Logger) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &Client{
		cli:     cli,
		network: network,
		logger:  logger,
	}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

func (c *Client) PullImage(ctx context.Context, ref string) error {
	c.logger.Info("pulling image", "ref", ref)
	reader, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
	return nil
}

func (c *Client) StopAndRemove(ctx context.Context, name string) error {
	timeout := 10
	if err := c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout}); err != nil {
		c.logger.Warn("stop container", "name", name, "error", err)
	}
	return c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
}

func (c *Client) StartContainer(ctx context.Context, name, imageRef string, internalPort int) (string, error) {
	c.EnsureNetwork(ctx)

	config := &container.Config{
		Image: imageRef,
		ExposedPorts: nat.PortSet{
			nat.Port(fmt.Sprintf("%d/tcp", internalPort)): struct{}{},
		},
		Labels: map[string]string{
			"managed-by": "talos",
		},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		NetworkMode: container.NetworkMode(c.network),
	}

	resp, err := c.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	c.logger.Info("container started", "id", resp.ID, "name", name)
	return resp.ID, nil
}

func (c *Client) WaitForHealth(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("health check timeout after %s", timeout)
		case <-ticker.C:
			info, err := c.cli.ContainerInspect(ctx, containerID)
			if err != nil {
				return fmt.Errorf("inspect container: %w", err)
			}

			if info.State.Running {
				if info.State.Health == nil {
					return nil
				}
				switch info.State.Health.Status {
				case "healthy":
					return nil
				case "unhealthy":
					return fmt.Errorf("container is unhealthy")
				}
			} else {
				return fmt.Errorf("container is not running")
			}
		}
	}
}

func (c *Client) GetLogs(ctx context.Context, containerID string, tail string) (string, error) {
	reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}
	return string(data), nil
}

// StreamLogs returns a streaming reader for container logs.
// The caller must close the returned reader when done.
// The Docker stream uses a multiplexed format: 8-byte header per frame
// (byte 0 = stream type 1=stdout 2=stderr, bytes 4-7 = uint32 big-endian payload size).
func (c *Client) StreamLogs(ctx context.Context, containerID string, tail string) (io.ReadCloser, error) {
	reader, err := c.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
		Follow:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("stream logs: %w", err)
	}
	return reader, nil
}

func (c *Client) EnsureNetwork(ctx context.Context) error {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == c.network {
			return nil
		}
	}

	_, err = c.cli.NetworkCreate(ctx, c.network, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"managed-by": "talos"},
	})
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}

	c.logger.Info("created network", "name", c.network)
	return nil
}
