package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"

	"monitoring/internal/models"
	sshclient "monitoring/internal/ssh"
	"monitoring/internal/utils"
)

// SFTPClient wraps the SFTP client with additional functionality
type SFTPClient struct {
	sshClient  *sshclient.SSHClient
	sftpClient *sftp.Client
	mu         sync.Mutex
}

// SFTPPool manages a pool of SFTP connections
type SFTPPool struct {
	clients map[uint]*SFTPClient
	mu      sync.RWMutex
}

var Pool *SFTPPool

func InitPool() {
	Pool = &SFTPPool{
		clients: make(map[uint]*SFTPClient),
	}
}

// GetClient returns an existing SFTP client or creates a new one
func (p *SFTPPool) GetClient(server *models.Server, password string) (*SFTPClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[server.ID]; exists {
		if client.sftpClient != nil {
			return client, nil
		}
	}

	// Get SSH client from pool
	sshClient, err := sshclient.Pool.GetClient(server, password)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH client: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sftp.NewClient(sshClient.GetUnderlyingClient())
	if err != nil {
		return nil, fmt.Errorf("failed to create SFTP client: %w", err)
	}

	client := &SFTPClient{
		sshClient:  sshClient,
		sftpClient: sftpClient,
	}

	p.clients[server.ID] = client
	utils.AppLogger.Info("SFTP client created for server %d", server.ID)

	return client, nil
}

// RemoveClient removes an SFTP client from the pool
func (p *SFTPPool) RemoveClient(serverID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[serverID]; exists {
		client.Close()
		delete(p.clients, serverID)
	}
}

// CloseAll closes all SFTP connections
func (p *SFTPPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, client := range p.clients {
		client.Close()
		delete(p.clients, id)
	}
}

// Close closes the SFTP connection
func (c *SFTPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sftpClient != nil {
		err := c.sftpClient.Close()
		c.sftpClient = nil
		return err
	}
	return nil
}

func (c *SFTPClient) ListDirectory(path string) ([]models.FileInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := c.sftpClient.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []models.FileInfo
	for _, entry := range entries {
		fileInfo := models.FileInfo{
			Name:        entry.Name(),
			Path:        filepath.Join(path, entry.Name()),
			Size:        entry.Size(),
			IsDir:       entry.IsDir(),
			Permissions: entry.Mode(),
			ModTime:     entry.ModTime(),
		}

		if stat, ok := entry.Sys().(*sftp.FileStat); ok {
			fileInfo.Owner = fmt.Sprintf("%d", stat.UID)
			fileInfo.Group = fmt.Sprintf("%d", stat.GID)
		}

		files = append(files, fileInfo)
	}

	return files, nil
}

// CreateDirectory creates a new directory
func (c *SFTPClient) CreateDirectory(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sftpClient.MkdirAll(path)
}

// RemoveDirectory removes a directory (recursively if needed)
func (c *SFTPClient) RemoveDirectory(path string, recursive bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !recursive {
		return c.sftpClient.RemoveDirectory(path)
	}

	return c.removeRecursive(path)
}

// removeRecursive removes a directory and all its contents
func (c *SFTPClient) removeRecursive(path string) error {
	entries, err := c.sftpClient.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if err := c.removeRecursive(fullPath); err != nil {
				return err
			}
		} else {
			if err := c.sftpClient.Remove(fullPath); err != nil {
				return err
			}
		}
	}

	return c.sftpClient.RemoveDirectory(path)
}

// UploadFile uploads a file to the remote server
func (c *SFTPClient) UploadFile(remotePath string, reader io.Reader, size int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure parent directory exists
	dir := filepath.Dir(remotePath)
	if err := c.sftpClient.MkdirAll(dir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// DownloadFile downloads a file from the remote server
func (c *SFTPClient) DownloadFile(remotePath string, writer io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return nil
}

// DeleteFile deletes a file
func (c *SFTPClient) DeleteFile(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sftpClient.Remove(path)
}

// Rename renames or moves a file/directory
func (c *SFTPClient) Rename(oldPath, newPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sftpClient.Rename(oldPath, newPath)
}

// ReadFileContent reads the content of a text file
func (c *SFTPClient) ReadFileContent(path string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := c.sftpClient.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// WriteFileContent writes content to a text file
func (c *SFTPClient) WriteFileContent(path, content string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := c.sftpClient.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = file.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Chmod changes file permissions
func (c *SFTPClient) Chmod(path string, mode os.FileMode) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sftpClient.Chmod(path, mode)
}

// Stat returns file information
func (c *SFTPClient) Stat(path string) (os.FileInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.sftpClient.Stat(path)
}

// SearchFiles searches for files matching a pattern
func (c *SFTPClient) SearchFiles(basePath, pattern string) ([]models.FileInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var results []models.FileInfo

	walker := c.sftpClient.Walk(basePath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}

		info := walker.Stat()
		name := info.Name()

		matched, err := filepath.Match(pattern, name)
		if err != nil {
			continue
		}

		if matched || strings.Contains(strings.ToLower(name), strings.ToLower(pattern)) {
			fileInfo := models.FileInfo{
				Name:        name,
				Path:        walker.Path(),
				Size:        info.Size(),
				IsDir:       info.IsDir(),
				Permissions: info.Mode(),
				ModTime:     info.ModTime(),
			}
			results = append(results, fileInfo)

			// Limit results to prevent overwhelming responses
			if len(results) >= 100 {
				break
			}
		}
	}

	return results, nil
}

// GetDirectorySize calculates the total size of a directory
func (c *SFTPClient) GetDirectorySize(path string) (*models.DirectorySizeResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := &models.DirectorySizeResult{
		Path: path,
	}

	walker := c.sftpClient.Walk(path)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}

		info := walker.Stat()
		if info.IsDir() {
			result.DirCount++
		} else {
			result.FileCount++
			result.Size += info.Size()
		}
	}

	return result, nil
}

// CopyFile copies a file within the server
func (c *SFTPClient) CopyFile(srcPath, dstPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	src, err := c.sftpClient.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Ensure parent directory exists
	dir := filepath.Dir(dstPath)
	if err := c.sftpClient.MkdirAll(dir); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	dst, err := c.sftpClient.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Copy permissions
	srcInfo, err := c.sftpClient.Stat(srcPath)
	if err == nil {
		c.sftpClient.Chmod(dstPath, srcInfo.Mode())
	}

	return nil
}

// Exists checks if a file or directory exists
func (c *SFTPClient) Exists(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, err := c.sftpClient.Stat(path)
	return err == nil
}
