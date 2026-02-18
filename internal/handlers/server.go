package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
	"monitoring/internal/models"
	"monitoring/internal/monitor"
	"monitoring/internal/utils"
)

// GetServers returns all servers
func GetServers(c *gin.Context) {
	var servers []models.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch servers"})
		return
	}

	dtos := make([]models.ServerDTO, len(servers))
	for i, server := range servers {
		dtos[i] = server.ToDTO()
	}

	c.JSON(http.StatusOK, gin.H{
		"servers": dtos,
		"total":   len(dtos),
	})
}

// GetServer returns a single server
func GetServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	var server models.Server
	if err := database.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	c.JSON(http.StatusOK, server.ToDTO())
}

// CreateServer creates a new server
func CreateServer(c *gin.Context) {
	var req models.CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encryptedPassword, err := utils.Encrypt(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
		return
	}

	if req.Port == "" {
		req.Port = "22"
	}
	if req.Sys == "" {
		req.Sys = models.SysLinux
	}
	if req.Connection == "" {
		req.Connection = models.ConnSSH
	}

	server := &models.Server{
		IPAddress:  req.IPAddress,
		Password:   encryptedPassword,
		Port:       req.Port,
		Sys:        req.Sys,
		Connection: req.Connection,
		Username:   req.Username,
		Name:       req.Name,
		Status:     models.StatusOffline,
	}

	if err := database.DB.Create(server).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create server"})
		return
	}

	// Start monitoring worker
	if err := monitor.Pool.AddWorker(server, req.Password); err != nil {
		utils.AppLogger.Warning("Failed to start monitoring: %v", err)
	}

	c.JSON(http.StatusCreated, server.ToDTO())
}

// UpdateServer updates an existing server
func UpdateServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	var server models.Server
	if err := database.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	var req models.UpdateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.IPAddress != "" {
		server.IPAddress = req.IPAddress
	}
	if req.Password != "" {
		encryptedPassword, err := utils.Encrypt(req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
			return
		}
		server.Password = encryptedPassword
	}
	if req.Port != "" {
		server.Port = req.Port
	}
	if req.Sys != "" {
		server.Sys = req.Sys
	}
	if req.Connection != "" {
		server.Connection = req.Connection
	}
	if req.Username != "" {
		server.Username = req.Username
	}
	if req.Name != "" {
		server.Name = req.Name
	}

	if err := database.DB.Save(&server).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update server"})
		return
	}

	// Restart worker if credentials changed
	if req.Password != "" || req.IPAddress != "" || req.Port != "" || req.Username != "" {
		monitor.Pool.RemoveWorker(uint(id))
		password := req.Password
		if password == "" {
			password, _ = utils.Decrypt(server.Password)
		}
		monitor.Pool.AddWorker(&server, password)
	}

	c.JSON(http.StatusOK, server.ToDTO())
}

// DeleteServer deletes a server
func DeleteServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	monitor.Pool.RemoveWorker(uint(id))

	if err := database.DB.Delete(&models.Server{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete server"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Server deleted"})
}

// GetServerStatus returns the current status of a server
func GetServerStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	var server models.Server
	if err := database.DB.First(&server, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"server_id":     id,
		"status":        server.Status,
		"is_monitoring": monitor.Pool.GetWorkerStatus(uint(id)),
	})
}
