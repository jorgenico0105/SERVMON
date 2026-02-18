package models

import (
	"os"
	"time"
)

// FileInfo represents a file/directory in SFTP listing
type FileInfo struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Size        int64       `json:"size"`
	IsDir       bool        `json:"is_dir"`
	Permissions os.FileMode `json:"permissions"`
	ModTime     time.Time   `json:"mod_time"`
	Owner       string      `json:"owner"`
	Group       string      `json:"group"`
}

// DirectoryRequest for creating directories
type DirectoryRequest struct {
	Path string `json:"path" binding:"required"`
}

// RenameRequest for renaming/moving files
type RenameRequest struct {
	OldPath string `json:"old_path" binding:"required"`
	NewPath string `json:"new_path" binding:"required"`
}

// DeleteRequest for deleting files/directories
type DeleteRequest struct {
	Path      string `json:"path" binding:"required"`
	Recursive bool   `json:"recursive"`
}

// ContentRequest for reading/writing file content
type ContentRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content"`
}

// ChmodRequest for changing file permissions
type ChmodRequest struct {
	Path       string      `json:"path" binding:"required"`
	Permission os.FileMode `json:"permission" binding:"required"`
}

// SearchRequest for searching files
type SearchRequest struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern" binding:"required"`
}

// SearchResult represents a search result
type SearchResult struct {
	Files []FileInfo `json:"files"`
	Total int        `json:"total"`
}

// DirectorySizeResult for directory size
type DirectorySizeResult struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	FileCount  int    `json:"file_count"`
	DirCount   int    `json:"dir_count"`
}
