package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
	"monitoring/internal/models"
	"monitoring/internal/ssh"
	"monitoring/internal/utils"
)

type ExecuteCommandRequest struct {
	Command string `json:"command" binding:"required"`
}

func ConnectServerSsh(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("serverId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}
	var server models.Server
	if err := database.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

}

// ExecuteSSHCommand executes a command on a server via SSH
func ExecuteSSHCommand(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("serverId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid server ID"})
		return
	}

	var req ExecuteCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var server models.Server
	if err := database.DB.First(&server, serverID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found"})
		return
	}

	password, err := utils.Decrypt(server.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decrypt credentials"})
		return
	}

	client, err := ssh.Pool.GetClient(&server, password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to server"})
		return
	}

	var fullCommand string
	if client.CurrentDir != "" {
		fullCommand = "cd " + client.CurrentDir + " && " + req.Command
	} else {
		fullCommand = req.Command
	}

	utils.AppLogger.Info("Comando ejecutado: %s", fullCommand)
	output, err := client.Execute(fullCommand)

	if err == nil && strings.HasPrefix(strings.TrimSpace(req.Command), "cd ") {
		var pwdCmd string
		if client.CurrentDir != "" {
			pwdCmd = "cd " + client.CurrentDir + " && " + req.Command + " && pwd"
		} else {
			pwdCmd = req.Command + " && pwd"
		}
		if newDir, pwdErr := client.Execute(pwdCmd); pwdErr == nil {
			client.CurrentDir = strings.TrimSpace(newDir)
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "Command failed",
			"detail": err.Error(),
		})
		return
	}

	// Format output as array of lines for better readability
	lines := strings.Split(strings.TrimSpace(output), "\n")

	c.JSON(http.StatusOK, gin.H{
		"output":     output,
		"lines":      lines,
		"command":    req.Command,
		"currentDir": client.CurrentDir,
	})
}
