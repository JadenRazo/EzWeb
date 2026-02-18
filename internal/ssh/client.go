package sshutil

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// NewClient establishes an SSH connection to the given host using public key
// authentication. The connection has a 10-second timeout.
func NewClient(host string, port int, user string, keyPath string) (*ssh.Client, error) {
	return NewClientWithHostKey(host, port, user, keyPath, "")
}

func NewClientWithHostKey(host string, port int, user string, keyPath string, knownHostKey string) (*ssh.Client, error) {
	signer, err := LoadPrivateKey(keyPath)
	if err != nil {
		return nil, err
	}

	var hostKeyCallback ssh.HostKeyCallback
	if knownHostKey != "" {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(knownHostKey))
		if err != nil {
			return nil, fmt.Errorf("invalid stored host key: %w", err)
		}
		hostKeyCallback = ssh.FixedHostKey(pubKey)
	} else {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	return client, nil
}

func GetHostKey(host string, port int) (string, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	var hostKey ssh.PublicKey

	config := &ssh.ClientConfig{
		User: "probe",
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			hostKey = key
			return nil
		},
		Timeout: 10 * time.Second,
	}

	conn, err := ssh.Dial("tcp", addr, config)
	if conn != nil {
		conn.Close()
	}
	if hostKey != nil {
		return string(ssh.MarshalAuthorizedKey(hostKey)), nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get host key: %w", err)
	}
	return "", fmt.Errorf("no host key received")
}

// RunCommand executes a single command on the remote host and returns
// the combined stdout+stderr output.
func RunCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return strings.TrimSpace(string(output)), fmt.Errorf("command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// TestConnection verifies SSH access and checks for a running Docker daemon
// by executing `docker info` on the remote host. Returns the Docker server
// version string on success.
func TestConnection(host string, port int, user string, keyPath string) (string, error) {
	client, err := NewClient(host, port, user, keyPath)
	if err != nil {
		return "", err
	}
	defer client.Close()

	version, err := RunCommand(client, "docker info --format '{{.ServerVersion}}'")
	if err != nil {
		return "", fmt.Errorf("docker not available: %w", err)
	}
	return version, nil
}
