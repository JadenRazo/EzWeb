package docker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func runCompose(ctx context.Context, composePath string, args ...string) (string, error) {
	cmdArgs := append([]string{"compose", "-f", composePath + "/docker-compose.yml"}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)
	cmd.Dir = composePath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker compose %s failed in %s: %w\n%s", strings.Join(args, " "), composePath, err, string(out))
	}
	return string(out), nil
}

func LocalComposeUp(ctx context.Context, composePath string) error {
	_, err := runCompose(ctx, composePath, "up", "-d")
	return err
}

func LocalComposeStop(ctx context.Context, composePath string) error {
	_, err := runCompose(ctx, composePath, "stop")
	return err
}

func LocalComposeStart(ctx context.Context, composePath string) error {
	_, err := runCompose(ctx, composePath, "start")
	return err
}

func LocalComposeRestart(ctx context.Context, composePath string) error {
	_, err := runCompose(ctx, composePath, "restart")
	return err
}

func LocalComposeDown(ctx context.Context, composePath string) error {
	_, err := runCompose(ctx, composePath, "down")
	return err
}

func LocalComposeLogs(ctx context.Context, composePath string, tail int) (string, error) {
	return runCompose(ctx, composePath, "logs", "--tail", fmt.Sprintf("%d", tail), "--no-color")
}

func LocalComposePS(ctx context.Context, composePath string) (string, error) {
	return runCompose(ctx, composePath, "ps", "--format", "table")
}
