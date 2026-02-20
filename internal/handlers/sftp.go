package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
	"monitoring/internal/models"
	"monitoring/internal/sftp"
	"monitoring/internal/utils"
)

// getSFTPClient helper to get SFTP client for a server
func getSFTPClient(c *gin.Context) (*sftp.SFTPClient, error) {
	serverID, err := strconv.ParseUint(c.Param("serverId"), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid server ID")
	}

	var server models.Server
	if err := database.DB.First(&server, serverID).Error; err != nil {
		return nil, fmt.Errorf("server not found")
	}

	password, err := utils.Decrypt(server.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials")
	}

	client, err := sftp.Pool.GetClient(&server, password)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return client, nil
}

func ListFiles(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	path := c.DefaultQuery("path", "/")

	files, err := client.ListDirectory(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":  path,
		"files": files,
		"total": len(files),
	})
}

func CreateDirectory(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.DirectoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := client.CreateDirectory(req.Path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Directory created",
		"path":    req.Path,
	})
}

func UploadFile(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	defer file.Close()

	remotePath := c.PostForm("path")
	if remotePath == "" {
		remotePath = "/" + header.Filename
	} else {

		if filepath.Ext(remotePath) == "" {
			remotePath = filepath.Join(remotePath, header.Filename)
		}
	}

	if err := client.UploadFile(remotePath, file, header.Size); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "File uploaded",
		"path":     remotePath,
		"filename": header.Filename,
		"size":     header.Size,
	})
}

func DownloadFile(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Path is required"})
		return
	}

	// Get file info
	info, err := client.Stat(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot download a directory"})
		return
	}

	filename := filepath.Base(path)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))

	if err := client.DownloadFile(path, c.Writer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
}

func DeleteFile(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.DeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if it's a directory
	info, err := client.Stat(req.Path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if info.IsDir() {
		if err := client.RemoveDirectory(req.Path, req.Recursive); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := client.DeleteFile(req.Path); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Deleted successfully",
		"path":    req.Path,
	})
}

// RenameFile renames or moves a file
func RenameFile(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := client.Rename(req.OldPath, req.NewPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Renamed successfully",
		"old_path": req.OldPath,
		"new_path": req.NewPath,
	})
}

// ReadFileContent reads the content of a text file
func ReadFileContent(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Path is required"})
		return
	}

	info, err := client.Stat(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	if info.Size() > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 10MB)"})
		return
	}

	content, err := client.ReadFileContent(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":    path,
		"content": content,
		"size":    info.Size(),
	})
}

func WriteFileContent(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.ContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := client.WriteFileContent(req.Path, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "File saved",
		"path":    req.Path,
		"size":    len(req.Content),
	})
}

// SearchFiles searches for files matching a pattern
func SearchFiles(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pattern := c.Query("pattern")
	if pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pattern is required"})
		return
	}

	path := c.DefaultQuery("path", "/")

	files, err := client.SearchFiles(path, pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pattern": pattern,
		"path":    path,
		"files":   files,
		"total":   len(files),
	})
}

// GetDirectorySize returns the size of a directory
func GetDirectorySize(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Path is required"})
		return
	}

	result, err := client.GetDirectorySize(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ChangePermissions changes file permissions
func ChangePermissions(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req models.ChmodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := client.Chmod(req.Path, req.Permission); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Permissions changed",
		"path":       req.Path,
		"permission": req.Permission,
	})
}

// CopyFile copies a file within the server
func CopyFile(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Source      string `json:"source" binding:"required"`
		Destination string `json:"destination" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := client.CopyFile(req.Source, req.Destination); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "File copied",
		"source":      req.Source,
		"destination": req.Destination,
	})
}

// UploadFolder uploads a full folder preserving relative paths
func UploadFolder(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files provided"})
		return
	}

	basePath := c.PostForm("path")
	if basePath == "" {
		basePath = "/"
	}

	// relativePaths is parallel to files — contains webkitRelativePath values
	relativePaths := form.Value["paths"]

	var uploaded []string
	var failed []string

	for i, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			failed = append(failed, fileHeader.Filename)
			continue
		}

		var remotePath string
		if i < len(relativePaths) && relativePaths[i] != "" {
			remotePath = filepath.Join(basePath, relativePaths[i])
		} else {
			remotePath = filepath.Join(basePath, fileHeader.Filename)
		}

		if err := client.UploadFile(remotePath, file, fileHeader.Size); err != nil {
			failed = append(failed, fileHeader.Filename)
		} else {
			uploaded = append(uploaded, remotePath)
		}

		file.Close()
	}

	c.JSON(http.StatusOK, gin.H{
		"uploaded": uploaded,
		"failed":   failed,
		"total":    len(files),
	})
}

// UploadMultipleFiles uploads multiple files
func UploadMultipleFiles(c *gin.Context) {
	client, err := getSFTPClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files provided"})
		return
	}

	basePath := c.PostForm("path")
	if basePath == "" {
		basePath = "/"
	}

	var uploaded []string
	var failed []string

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			failed = append(failed, fileHeader.Filename)
			continue
		}

		remotePath := filepath.Join(basePath, fileHeader.Filename)
		if err := client.UploadFile(remotePath, file, fileHeader.Size); err != nil {
			failed = append(failed, fileHeader.Filename)
		} else {
			uploaded = append(uploaded, fileHeader.Filename)
		}

		file.Close()
	}

	c.JSON(http.StatusOK, gin.H{
		"uploaded": uploaded,
		"failed":   failed,
		"total":    len(files),
	})
}
