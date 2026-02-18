package models

import (
	"time"

	"gorm.io/gorm"
)

type ServerSys string
type ConnectionType string
type ServerStatus string

const (
	SysLinux   ServerSys = "L"
	SysWindows ServerSys = "W"

	ConnSSH   ConnectionType = "SSH"
	ConnWinRM ConnectionType = "WinRM"
	ConnSFTP  ConnectionType = "SFTP"

	StatusOnline  ServerStatus = "online"
	StatusOffline ServerStatus = "offline"
	StatusError   ServerStatus = "error"
)

type Server struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	IPAddress  string         `gorm:"column:ip_address;type:varchar(20);not null" json:"ip_address"`
	Password   string         `gorm:"type:varchar(255)" json:"-"`
	Port       string         `gorm:"type:varchar(10);default:'22'" json:"port"`
	Sys        ServerSys      `gorm:"type:varchar(1);default:'L'" json:"sys"`
	Connection ConnectionType `gorm:"type:varchar(10);default:'SSH'" json:"connection"`
	Username   string         `gorm:"type:varchar(50)" json:"username"`
	Name       string         `gorm:"type:varchar(100)" json:"name"`
	Status     ServerStatus   `gorm:"type:varchar(20);default:'offline'" json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Server) TableName() string {
	return "servers"
}

// ServerDTO for API responses
type ServerDTO struct {
	ID         uint           `json:"id"`
	IPAddress  string         `json:"ip_address"`
	Port       string         `json:"port"`
	Sys        ServerSys      `json:"sys"`
	Connection ConnectionType `json:"connection"`
	Username   string         `json:"username"`
	Name       string         `json:"name"`
	Status     ServerStatus   `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (s *Server) ToDTO() ServerDTO {
	return ServerDTO{
		ID:         s.ID,
		IPAddress:  s.IPAddress,
		Port:       s.Port,
		Sys:        s.Sys,
		Connection: s.Connection,
		Username:   s.Username,
		Name:       s.Name,
		Status:     s.Status,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}

// CreateServerRequest for API input
type CreateServerRequest struct {
	IPAddress  string         `json:"ip_address" binding:"required"`
	Password   string         `json:"password" binding:"required"`
	Port       string         `json:"port"`
	Sys        ServerSys      `json:"sys"`
	Connection ConnectionType `json:"connection"`
	Username   string         `json:"username" binding:"required"`
	Name       string         `json:"name" binding:"required"`
}

// UpdateServerRequest for API input
type UpdateServerRequest struct {
	IPAddress  string         `json:"ip_address"`
	Password   string         `json:"password"`
	Port       string         `json:"port"`
	Sys        ServerSys      `json:"sys"`
	Connection ConnectionType `json:"connection"`
	Username   string         `json:"username"`
	Name       string         `json:"name"`
}

// MetricSnapshot for real-time WebSocket broadcast (not stored in DB)
type MetricSnapshot struct {
	ServerID    uint    `json:"server_id"`
	ServerName  string  `json:"server_name"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemTotal    uint64  `json:"mem_total"`
	MemUsed     uint64  `json:"mem_used"`
	MemFree     uint64  `json:"mem_free"`
	MemPercent  float64 `json:"mem_percent"`
	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskFree    uint64  `json:"disk_free"`
	DiskPercent float64 `json:"disk_percent"`
	NetRX       uint64  `json:"net_rx"`
	NetTX       uint64  `json:"net_tx"`
	Uptime      uint64  `json:"uptime"`
	Timestamp   int64   `json:"timestamp"`
}
