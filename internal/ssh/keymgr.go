package sshutil

import (
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

// LoadPrivateKey reads an SSH private key from disk and parses it into a signer
// that can be used for public key authentication.
func LoadPrivateKey(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key at %s: %w", path, err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}
	return signer, nil
}
