package docker

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// NewLocalClient creates a Docker client connected to the local daemon,
// using environment variables and automatic API version negotiation.
func NewLocalClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// PingLocal verifies the local Docker daemon is reachable.
func PingLocal(ctx context.Context, cli *client.Client) error {
	_, err := cli.Ping(ctx)
	return err
}

// LocalServerInfo holds system info about the host machine running EzWeb.
type LocalServerInfo struct {
	Hostname        string
	OS              string
	Arch            string
	DockerVersion   string
	DockerStatus    string // "online" or "offline"
	ContainerCount  int
	RunningCount    int
}

// GetLocalServerInfo gathers system and Docker info for the localhost.
func GetLocalServerInfo(ctx context.Context) LocalServerInfo {
	info := LocalServerInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	hostname, err := os.Hostname()
	if err == nil {
		info.Hostname = hostname
	} else {
		info.Hostname = "localhost"
	}

	cli, err := NewLocalClient()
	if err != nil {
		info.DockerStatus = "offline"
		info.DockerVersion = "N/A"
		return info
	}
	defer cli.Close()

	sv, err := cli.ServerVersion(ctx)
	if err != nil {
		info.DockerStatus = "offline"
		info.DockerVersion = "N/A"
		return info
	}

	info.DockerStatus = "online"
	info.DockerVersion = fmt.Sprintf("Docker %s", sv.Version)

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err == nil {
		info.ContainerCount = len(containers)
		for _, c := range containers {
			if c.State == "running" {
				info.RunningCount++
			}
		}
	}

	return info
}
