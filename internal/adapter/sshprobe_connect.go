package adapter

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"specularium/internal/domain"
)

// connect establishes an SSH connection using the provided secret
// Supports both key-based and password authentication
func (s *SSHProbeAdapter) connect(ctx context.Context, host string, port int, secret *domain.Secret) (*ssh.Client, error) {
	// Build SSH client config based on secret type
	config, err := s.buildSSHConfig(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to build SSH config: %w", err)
	}

	// Apply timeout
	config.Timeout = s.timeout

	// Create address
	addr := fmt.Sprintf("%s:%d", host, port)

	// Create dialer with context support
	dialer := &net.Dialer{
		Timeout: s.timeout,
	}

	// Dial with context
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	// Create SSH connection from net.Conn
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to establish SSH connection: %w", err)
	}

	// Create SSH client
	client := ssh.NewClient(sshConn, chans, reqs)

	return client, nil
}

// buildSSHConfig creates an SSH client config from a secret
func (s *SSHProbeAdapter) buildSSHConfig(secret *domain.Secret) (*ssh.ClientConfig, error) {
	switch secret.Type {
	case domain.SecretTypeSSHKey:
		return s.buildSSHKeyConfig(secret)
	case domain.SecretTypeSSHPassword:
		return s.buildSSHPasswordConfig(secret)
	default:
		return nil, fmt.Errorf("unsupported secret type: %s", secret.Type)
	}
}

// buildSSHKeyConfig creates SSH config for key-based auth
func (s *SSHProbeAdapter) buildSSHKeyConfig(secret *domain.Secret) (*ssh.ClientConfig, error) {
	username := secret.Data["username"]
	if username == "" {
		return nil, fmt.Errorf("username not found in SSH key secret")
	}

	privateKeyData := secret.Data["private_key"]
	if privateKeyData == "" {
		return nil, fmt.Errorf("private_key not found in SSH key secret")
	}

	passphrase := secret.Data["passphrase"]

	// Parse private key
	var signer ssh.Signer
	var err error

	if passphrase != "" {
		// Encrypted key
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKeyData), []byte(passphrase))
	} else {
		// Unencrypted key
		signer, err = ssh.ParsePrivateKey([]byte(privateKeyData))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         s.timeout,
	}

	return config, nil
}

// buildSSHPasswordConfig creates SSH config for password auth
func (s *SSHProbeAdapter) buildSSHPasswordConfig(secret *domain.Secret) (*ssh.ClientConfig, error) {
	username := secret.Data["username"]
	if username == "" {
		return nil, fmt.Errorf("username not found in SSH password secret")
	}

	password := secret.Data["password"]
	if password == "" {
		return nil, fmt.Errorf("password not found in SSH password secret")
	}

	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         s.timeout,
	}

	return config, nil
}

// runCommand executes a command over SSH and returns the output
func (s *SSHProbeAdapter) runCommand(client *ssh.Client, cmd string) (string, error) {
	// Create session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Set timeout for command execution
	done := make(chan error, 1)
	var output []byte

	go func() {
		output, err = session.CombinedOutput(cmd)
		done <- err
	}()

	// Wait for command or timeout
	select {
	case err := <-done:
		if err != nil {
			// Check if it's a non-zero exit status - still return output
			if exitErr, ok := err.(*ssh.ExitError); ok {
				// Command ran but exited with non-zero status
				// Some commands (like docker ps when docker is not running) do this
				// We still want the output
				_ = exitErr // Ignore the exit error
				return string(output), nil
			}
			return "", fmt.Errorf("command failed: %w", err)
		}
		return string(output), nil
	case <-time.After(30 * time.Second): // Command timeout
		session.Signal(ssh.SIGKILL)
		return "", fmt.Errorf("command timeout")
	}
}
