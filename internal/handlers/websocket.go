package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"monitoring/internal/utils"
	ws "monitoring/internal/websocket"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// MonitorWebSocket handles WebSocket connections for real-time metrics
func MonitorWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		utils.AppLogger.Error("Failed to upgrade to WebSocket: %v", err)
		return
	}

	clientID := utils.GenerateID()
	client := ws.NewClient(clientID, conn, ws.Hub)

	ws.Hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
