package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"monitoring/config"
	"monitoring/internal/models"
	"monitoring/internal/utils"
)

type MessageType string

const (
	MessageTypeMetrics   MessageType = "server_metrics"
	MessageTypeStatus    MessageType = "server_status"
	MessageTypePing      MessageType = "ping"
	MessageTypePong      MessageType = "pong"
	MessageTypeSubscribe MessageType = "subscribe"
	MessageTypeError     MessageType = "error"
)

type Message struct {
	Type    MessageType `json:"type"`
	Payload interface{} `json:"payload"`
}

type Client struct {
	ID            string
	conn          *websocket.Conn
	hub           *WebSocketHub
	send          chan []byte
	subscriptions map[uint]bool
	mu            sync.Mutex
}

type WebSocketHub struct {
	clients    map[*Client]bool
	rooms      map[uint]map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

var Hub *WebSocketHub

func InitHub() {
	Hub = &WebSocketHub{
		clients:    make(map[*Client]bool),
		rooms:      make(map[uint]map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			utils.AppLogger.Info("WebSocket client connected: %s", client.ID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				for serverID := range client.subscriptions {
					if room, exists := h.rooms[serverID]; exists {
						delete(room, client)
					}
				}
			}
			h.mu.Unlock()
			utils.AppLogger.Info("WebSocket client disconnected: %s", client.ID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastMetrics sends metrics to all connected clients
func (h *WebSocketHub) BroadcastMetrics(metrics *models.MetricSnapshot) {
	msg := Message{
		Type:    MessageTypeMetrics,
		Payload: metrics,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		utils.AppLogger.Error("Failed to marshal metrics: %v", err)
		return
	}

	h.broadcast <- data
	h.broadcastToRoom(metrics.ServerID, data)
}

// BroadcastServerStatus broadcasts a server status change
func (h *WebSocketHub) BroadcastServerStatus(serverID uint, status models.ServerStatus) {
	msg := Message{
		Type: MessageTypeStatus,
		Payload: map[string]interface{}{
			"server_id": serverID,
			"status":    status,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.broadcast <- data
}

func (h *WebSocketHub) broadcastToRoom(serverID uint, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if room, exists := h.rooms[serverID]; exists {
		for client := range room {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

func (h *WebSocketHub) Subscribe(client *Client, serverID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.rooms[serverID]; !exists {
		h.rooms[serverID] = make(map[*Client]bool)
	}

	h.rooms[serverID][client] = true
	client.mu.Lock()
	client.subscriptions[serverID] = true
	client.mu.Unlock()
}

func (h *WebSocketHub) Unsubscribe(client *Client, serverID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, exists := h.rooms[serverID]; exists {
		delete(room, client)
	}

	client.mu.Lock()
	delete(client.subscriptions, serverID)
	client.mu.Unlock()
}

func NewClient(id string, conn *websocket.Conn, hub *WebSocketHub) *Client {
	return &Client{
		ID:            id,
		conn:          conn,
		hub:           hub,
		send:          make(chan []byte, 256),
		subscriptions: make(map[uint]bool),
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(config.AppConfig.WSPongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(config.AppConfig.WSPongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				utils.AppLogger.Error("WebSocket error: %v", err)
			}
			break
		}
		c.handleMessage(message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(config.AppConfig.WSPingInterval)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(data []byte) {
	var msg struct {
		Type     MessageType `json:"type"`
		ServerID uint        `json:"server_id,omitempty"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		c.sendError("Invalid message format")
		return
	}

	switch msg.Type {
	case MessageTypeSubscribe:
		if msg.ServerID > 0 {
			c.hub.Subscribe(c, msg.ServerID)
			c.sendAck("subscribed", msg.ServerID)
		}
	case MessageTypePing:
		c.sendPong()
	}
}

func (c *Client) sendError(message string) {
	msg := Message{
		Type:    MessageTypeError,
		Payload: map[string]string{"error": message},
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *Client) sendAck(action string, serverID uint) {
	msg := Message{
		Type: "ack",
		Payload: map[string]interface{}{
			"action":    action,
			"server_id": serverID,
		},
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *Client) sendPong() {
	msg := Message{Type: MessageTypePong}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (h *WebSocketHub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *WebSocketHub) Register(client *Client) {
	h.register <- client
}
