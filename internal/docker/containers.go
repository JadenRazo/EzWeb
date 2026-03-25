package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// ListContainers returns all containers (including stopped) from the local Docker daemon.
func ListContainers(ctx context.Context, cli *client.Client) ([]types.Container, error) {
	return cli.ContainerList(ctx, container.ListOptions{All: true})
}

// StopContainer stops a running container by its ID or name.
func StopContainer(ctx context.Context, cli *client.Client, containerID string) error {
	return cli.ContainerStop(ctx, containerID, container.StopOptions{})
}

// StartContainer starts a stopped container by its ID or name.
func StartContainer(ctx context.Context, cli *client.Client, containerID string) error {
	return cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// RestartContainer restarts a container by its ID or name.
func RestartContainer(ctx context.Context, cli *client.Client, containerID string) error {
	return cli.ContainerRestart(ctx, containerID, container.StopOptions{})
}

// GetContainerLogs retrieves the last N lines of logs from a container.
func GetContainerLogs(ctx context.Context, cli *client.Client, containerID string, tail string) (string, error) {
	reader, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
