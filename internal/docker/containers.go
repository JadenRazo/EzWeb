package docker

import (
	"context"
	"io"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// ListContainers returns all containers (including stopped) from the local Docker daemon.
func ListContainers(ctx context.Context, cli *client.Client) ([]container.Summary, error) {
	res, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

// StopContainer stops a running container by its ID or name.
func StopContainer(ctx context.Context, cli *client.Client, containerID string) error {
	_, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{})
	return err
}

// StartContainer starts a stopped container by its ID or name.
func StartContainer(ctx context.Context, cli *client.Client, containerID string) error {
	_, err := cli.ContainerStart(ctx, containerID, client.ContainerStartOptions{})
	return err
}

// RestartContainer restarts a container by its ID or name.
func RestartContainer(ctx context.Context, cli *client.Client, containerID string) error {
	_, err := cli.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{})
	return err
}

// GetContainerLogs retrieves the last N lines of logs from a container.
func GetContainerLogs(ctx context.Context, cli *client.Client, containerID string, tail string) (string, error) {
	reader, err := cli.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
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
