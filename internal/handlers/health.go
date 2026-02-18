package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"monitoring/internal/database"
)

var startTime = time.Now()

func HealthCheck(c *gin.Context) {
	dbStatus := "ok"
	sqlDB, err := database.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		dbStatus = "error"
	}

	status := "healthy"
	if dbStatus == "error" {
		status = "unhealthy"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"uptime":    time.Since(startTime).String(),
		"database":  dbStatus,
		"timestamp": time.Now().Unix(),
	})
}

// ReadyCheck returns whether the app is ready
func ReadyCheck(c *gin.Context) {
	sqlDB, err := database.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ready": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ready": true})
}
