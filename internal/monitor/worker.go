package monitor

import (
	"context"
	"sync"
	"time"

	"monitoring/config"
	"monitoring/internal/database"
	"monitoring/internal/models"
	"monitoring/internal/ssh"
	"monitoring/internal/utils"
	"monitoring/internal/websocket"
)

// Worker monitors a single server
type Worker struct {
	server    *models.Server
	password  string
	sshClient *ssh.SSHClient
	collector *ssh.MetricCollector
	ctx       context.Context
	cancel    context.CancelFunc
	logger    *utils.ContextLogger
	running   bool
	mu        sync.Mutex
}

// WorkerPool manages all monitoring workers
type WorkerPool struct {
	workers map[uint]*Worker
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

var Pool *WorkerPool

// InitWorkerPool initializes the worker pool
func InitWorkerPool() {
	ctx, cancel := context.WithCancel(context.Background())
	Pool = &WorkerPool{
		workers: make(map[uint]*Worker),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// StartAll starts monitoring for all active servers
func (p *WorkerPool) StartAll() error {
	var servers []models.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		return err
	}

	for _, server := range servers {
		password, err := utils.Decrypt(server.Password)
		if err != nil {
			utils.AppLogger.Error("Failed to decrypt password for server %d: %v", server.ID, err)
			continue
		}
		if err := p.AddWorker(&server, password); err != nil {
			utils.AppLogger.Error("Failed to start worker for server %d: %v", server.ID, err)
		}
	}

	return nil
}

// AddWorker adds a new worker for a server
func (p *WorkerPool) AddWorker(server *models.Server, password string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.workers[server.ID]; exists {
		return nil
	}

	ctx, cancel := context.WithCancel(p.ctx)
	worker := &Worker{
		server:   server,
		password: password,
		ctx:      ctx,
		cancel:   cancel,
		logger:   utils.AppLogger.WithContext(server.ID, server.Name),
	}

	p.workers[server.ID] = worker
	go worker.Run()

	utils.AppLogger.Info("Started monitoring worker for server %d (%s)", server.ID, server.Name)
	return nil
}

// RemoveWorker stops and removes a worker
func (p *WorkerPool) RemoveWorker(serverID uint) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if worker, exists := p.workers[serverID]; exists {
		worker.Stop()
		delete(p.workers, serverID)
		utils.AppLogger.Info("Stopped monitoring worker for server %d", serverID)
	}
}

// StopAll stops all workers
func (p *WorkerPool) StopAll() {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	for id, worker := range p.workers {
		worker.Stop()
		delete(p.workers, id)
	}

	utils.AppLogger.Info("Stopped all monitoring workers")
}

// GetWorkerStatus returns the status of a worker
func (p *WorkerPool) GetWorkerStatus(serverID uint) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if worker, exists := p.workers[serverID]; exists {
		return worker.IsRunning()
	}
	return false
}

// Run starts the worker's monitoring loop
func (w *Worker) Run() {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	// Initial connection attempt
	if err := w.connect(); err != nil {
		w.logger.Error("Initial connection failed: %v", err)
		w.updateServerStatus(models.StatusError)
	} else {
		w.updateServerStatus(models.StatusOnline)
	}

	ticker := time.NewTicker(config.AppConfig.MetricsInterval)
	defer ticker.Stop()

	reconnectAttempts := 0
	maxReconnectAttempts := 3

	for {
		select {
		case <-w.ctx.Done():
			w.logger.Info("Worker stopping")
			return
		case <-ticker.C:
			if w.sshClient == nil || !w.sshClient.IsConnected() {
				reconnectAttempts++
				if reconnectAttempts > maxReconnectAttempts {
					w.logger.Error("Max reconnect attempts reached")
					w.updateServerStatus(models.StatusError)
					reconnectAttempts = 0
					time.Sleep(30 * time.Second)
					continue
				}

				w.logger.Warning("Connection lost, reconnecting (%d/%d)", reconnectAttempts, maxReconnectAttempts)
				if err := w.connect(); err != nil {
					w.logger.Error("Reconnection failed: %v", err)
					w.updateServerStatus(models.StatusError)
					continue
				}
				reconnectAttempts = 0
				w.updateServerStatus(models.StatusOnline)
			}

			// Collect and broadcast metrics (no database storage)
			metrics, err := w.collector.CollectAll()
			if err != nil {
				w.logger.Error("Failed to collect metrics: %v", err)
				continue
			}

			// Broadcast via WebSocket only
			websocket.Hub.BroadcastMetrics(metrics)
		}
	}
}

// connect establishes SSH connection
func (w *Worker) connect() error {
	client, err := ssh.Pool.GetClient(w.server, w.password)
	if err != nil {
		return err
	}

	w.sshClient = client
	w.collector = ssh.NewMetricCollector(client)
	return nil
}

// updateServerStatus updates the server status in database
func (w *Worker) updateServerStatus(status models.ServerStatus) {
	w.server.Status = status
	database.DB.Model(&models.Server{}).Where("id = ?", w.server.ID).Update("status", status)
}

// Stop stops the worker
func (w *Worker) Stop() {
	w.cancel()
	if w.sshClient != nil {
		ssh.Pool.RemoveClient(w.server.ID)
	}
}

// IsRunning returns whether the worker is running
func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
