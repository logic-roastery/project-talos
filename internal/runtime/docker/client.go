package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type Client struct {
	cli     *client.Client
	network string
	logger  *slog.Logger
}

type ContainerInfo struct {
	ID       string
	Name     string
	Image    string
	State    string
	Status   string
	Networks []string
	Ports    []string
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
	reader, err := c.cli.ImagePull(ctx, ref, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", ref, err)
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)
	return nil
}

func (c *Client) StopAndRemove(ctx context.Context, name string) error {
	timeout := 10
	if _, err := c.cli.ContainerStop(ctx, name, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		c.logger.Warn("stop container", "name", name, "error", err)
	}
	_, err := c.cli.ContainerRemove(ctx, name, client.ContainerRemoveOptions{Force: true})
	return err
}

func (c *Client) Restart(ctx context.Context, name string) error {
	_, err := c.cli.ContainerRestart(ctx, name, client.ContainerRestartOptions{})
	if err != nil {
		return fmt.Errorf("restart container: %w", err)
	}
	return nil
}

func (c *Client) Inspect(ctx context.Context, name string) (container.InspectResponse, error) {
	result, err := c.cli.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}
	return result.Container, nil
}

func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error) {
	result, err := c.cli.ContainerList(ctx, client.ContainerListOptions{All: all})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	items := make([]ContainerInfo, 0, len(result.Items))
	for _, item := range result.Items {
		info := ContainerInfo{
			ID:     item.ID,
			Image:  item.Image,
			State:  string(item.State),
			Status: item.Status,
		}
		if len(item.Names) > 0 {
			info.Name = strings.TrimPrefix(item.Names[0], "/")
		}
		for _, port := range item.Ports {
			privatePort := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
			if port.PublicPort > 0 {
				hostIP := ""
				if port.IP.IsValid() {
					hostIP = port.IP.String()
				}
				if hostIP != "" {
					info.Ports = append(info.Ports, fmt.Sprintf("%s:%d->%s", hostIP, port.PublicPort, privatePort))
				} else {
					info.Ports = append(info.Ports, fmt.Sprintf("%d->%s", port.PublicPort, privatePort))
				}
			} else {
				info.Ports = append(info.Ports, privatePort)
			}
		}
		for networkName := range item.NetworkSettings.Networks {
			info.Networks = append(info.Networks, networkName)
		}
		items = append(items, info)
	}

	return items, nil
}

func (c *Client) StartContainer(ctx context.Context, name, imageRef string, internalPort int) (string, error) {
	if err := c.EnsureNetwork(ctx); err != nil {
		return "", err
	}

	port, err := network.ParsePort(fmt.Sprintf("%d/tcp", internalPort))
	if err != nil {
		return "", fmt.Errorf("parse internal port: %w", err)
	}

	config := &container.Config{
		Image: imageRef,
		ExposedPorts: network.PortSet{
			port: struct{}{},
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

	resp, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostConfig,
		Name:       name,
	})
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if _, err := c.cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	c.logger.Info("container started", "id", resp.ID, "name", name)
	return resp.ID, nil
}

// ContainerConfig holds advanced options for creating a container.
type ContainerConfig struct {
	Name         string
	ImageRef     string
	InternalPort int
	Env          []string // KEY=VALUE pairs
	Volumes      []string // hostPath:containerPath
	Ports        []string // host:container bindings, e.g. "80:80", "443:443"
	Networks     []string // additional Docker networks to connect after start
	ExtraHosts   []string // host mappings, e.g. "host.docker.internal:host-gateway"
	HealthCheck  *container.HealthConfig
	Labels       map[string]string
}

// StartContainerWithConfig creates and starts a container with full configuration
// (volumes, env vars, custom health check, extra labels).
func (c *Client) StartContainerWithConfig(ctx context.Context, cfg ContainerConfig) (string, error) {
	if err := c.EnsureNetwork(ctx); err != nil {
		return "", err
	}

	labels := map[string]string{
		"managed-by": "talos",
	}
	for k, v := range cfg.Labels {
		labels[k] = v
	}

	exposedPorts := network.PortSet{}
	if cfg.InternalPort > 0 {
		port, err := network.ParsePort(fmt.Sprintf("%d/tcp", cfg.InternalPort))
		if err != nil {
			return "", fmt.Errorf("parse internal port: %w", err)
		}
		exposedPorts[port] = struct{}{}
	}

	portBindings := network.PortMap{}
	if len(cfg.Ports) > 0 {
		eps, bindings, err := nat.ParsePortSpecs(cfg.Ports)
		if err != nil {
			return "", fmt.Errorf("parse port specs: %w", err)
		}
		for p := range eps {
			port, err := network.ParsePort(string(p))
			if err != nil {
				return "", fmt.Errorf("parse exposed port %s: %w", p, err)
			}
			exposedPorts[port] = struct{}{}
		}
		for p, hostBindings := range bindings {
			port, err := network.ParsePort(string(p))
			if err != nil {
				return "", fmt.Errorf("parse bound port %s: %w", p, err)
			}

			converted := make([]network.PortBinding, 0, len(hostBindings))
			for _, binding := range hostBindings {
				hostIP, err := parseHostIP(binding.HostIP)
				if err != nil {
					return "", fmt.Errorf("parse host IP %q: %w", binding.HostIP, err)
				}
				converted = append(converted, network.PortBinding{
					HostIP:   hostIP,
					HostPort: binding.HostPort,
				})
			}

			portBindings[port] = converted
		}
	}

	config := &container.Config{
		Image:        cfg.ImageRef,
		ExposedPorts: exposedPorts,
		Env:          cfg.Env,
		Labels:       labels,
	}
	if cfg.HealthCheck != nil {
		config.Healthcheck = cfg.HealthCheck
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		NetworkMode:  container.NetworkMode(c.network),
		Binds:        cfg.Volumes,
		PortBindings: portBindings,
		ExtraHosts:   cfg.ExtraHosts,
	}

	resp, err := c.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostConfig,
		Name:       cfg.Name,
	})
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if _, err := c.cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	for _, networkName := range cfg.Networks {
		if networkName == "" || networkName == c.network {
			continue
		}
		if err := c.ConnectContainerToNetwork(ctx, resp.ID, networkName); err != nil {
			return "", fmt.Errorf("connect container to network %s: %w", networkName, err)
		}
	}

	c.logger.Info("container started", "id", resp.ID, "name", cfg.Name)
	return resp.ID, nil
}

func (c *Client) ConnectContainerToNetwork(ctx context.Context, containerID, networkName string) error {
	networks, err := c.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	exists := false
	for _, n := range networks.Items {
		if n.Name == networkName {
			exists = true
			break
		}
	}
	if !exists {
		_, err := c.cli.NetworkCreate(ctx, networkName, client.NetworkCreateOptions{
			Driver: "bridge",
		})
		if err != nil {
			return fmt.Errorf("create network %s: %w", networkName, err)
		}
	}

	_, err = c.cli.NetworkConnect(ctx, networkName, client.NetworkConnectOptions{
		Container: containerID,
	})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("network connect %s: %w", networkName, err)
		}
	}
	return nil
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
			info, err := c.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
			if err != nil {
				return fmt.Errorf("inspect container: %w", err)
			}

			if info.Container.State.Running {
				if info.Container.State.Health == nil {
					return nil
				}
				switch info.Container.State.Health.Status {
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
	reader, err := c.cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
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
	reader, err := c.cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
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

// Exec runs a command in a running container and returns the combined stdout output.
func (c *Client) Exec(ctx context.Context, containerName string, cmd []string) ([]byte, error) {
	execCfg := client.ExecCreateOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	resp, err := c.cli.ExecCreate(ctx, containerName, execCfg)
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := c.cli.ExecAttach(ctx, resp.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	output, err := io.ReadAll(attachResp.Reader)
	if err != nil {
		return nil, fmt.Errorf("exec read output: %w", err)
	}

	inspectResp, err := c.cli.ExecInspect(ctx, resp.ID, client.ExecInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}
	if inspectResp.ExitCode != 0 {
		return output, fmt.Errorf("exec exited with code %d", inspectResp.ExitCode)
	}

	return output, nil
}

func (c *Client) EnsureNetwork(ctx context.Context) error {
	networks, err := c.cli.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	for _, n := range networks.Items {
		if n.Name == c.network {
			return nil
		}
	}

	_, err = c.cli.NetworkCreate(ctx, c.network, client.NetworkCreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"managed-by": "talos"},
	})
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}

	c.logger.Info("created network", "name", c.network)
	return nil
}

func parseHostIP(value string) (netip.Addr, error) {
	if value == "" {
		return netip.Addr{}, nil
	}
	return netip.ParseAddr(value)
}
