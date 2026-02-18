package docker

import (
	"context"

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
