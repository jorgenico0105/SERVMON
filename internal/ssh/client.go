package ssh

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"monitoring/config"
	"monitoring/internal/models"
	"monitoring/internal/utils"
)

// SSHClient manages SSH connections to a server
type SSHClient struct {
	Server     *models.Server
	client     *ssh.Client
	mu         sync.Mutex
	connected  bool
	lastUsed   time.Time
	password   string // Decrypted password
	CurrentDir string // Current working directory
}

// SSHPool manages a pool of SSH connections
type SSHPool struct {
	clients map[uint]*SSHClient
	mu      sync.RWMutex
}

var Pool *SSHPool

func InitPool() {
	Pool = &SSHPool{
		clients: make(map[uint]*SSHClient),
	}
}

func (p *SSHPool) GetClient(server *models.Server, password string) (*SSHClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[server.ID]; exists && client.connected {
		client.lastUsed = time.Now()
		return client, nil
	}

	client := &SSHClient{
		Server:   server,
		password: password,
	}

	if err := client.Connect(); err != nil {
		return nil, err
	}

	p.clients[server.ID] = client
	return client, nil
}

// RemoveClient removes a client from the pool
func (p *SSHPool) RemoveClient(serverID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[serverID]; exists {
		client.Close()
		delete(p.clients, serverID)
	}
}

// CloseAll closes all connections in the pool
func (p *SSHPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, client := range p.clients {
		client.Close()
		delete(p.clients, id)
	}
}

// Connect establishes SSH connection
func (c *SSHClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected && c.client != nil {
		return nil
	}

	sshConfig := &ssh.ClientConfig{
		User: c.Server.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		Timeout:         config.AppConfig.SSHTimeout,
	}

	addr := fmt.Sprintf("%s:%s", c.Server.IPAddress, c.Server.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		utils.AppLogger.Error("SSH connection failed to %s: %v", addr, err)
		return fmt.Errorf("ssh dial failed: %w", err)
	}

	c.client = client
	c.connected = true
	c.lastUsed = time.Now()

	utils.AppLogger.Info("SSH connected to %s", addr)
	return nil
}

// Close closes the SSH connection
func (c *SSHClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		c.connected = false
		return err
	}
	return nil
}

// IsConnected checks if the client is connected
func (c *SSHClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && c.client != nil
}

// Execute runs a command on the remote server
func (c *SSHClient) Execute(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		c.connected = false
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("command failed: %s", stderr.String())
		}
		return "", fmt.Errorf("command failed: %w", err)
	}

	c.lastUsed = time.Now()
	return stdout.String(), nil
}

// ExecuteWithTimeout runs a command with a specific timeout
func (c *SSHClient) ExecuteWithTimeout(command string, timeout time.Duration) (string, error) {
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := c.Execute(command)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return "", err
	case <-time.After(timeout):
		return "", fmt.Errorf("command timeout after %v", timeout)
	}
}

// Reconnect attempts to reconnect to the server
func (c *SSHClient) Reconnect() error {
	c.Close()
	return c.Connect()
}

// GetUnderlyingClient returns the raw SSH client for advanced operations
func (c *SSHClient) GetUnderlyingClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

// TestConnection tests if the connection is still alive
func (c *SSHClient) TestConnection() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.client == nil {
		return fmt.Errorf("not connected")
	}

	// Send a keepalive request
	_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
	if err != nil {
		c.connected = false
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}
