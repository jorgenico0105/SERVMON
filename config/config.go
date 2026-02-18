package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	ServerPort string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// SSH
	SSHTimeout   time.Duration
	SSHKeepAlive time.Duration

	// Monitoring
	MetricsInterval time.Duration

	// Security
	EncryptionKey string

	// WebSocket
	WSPingInterval time.Duration
	WSPongWait     time.Duration
}

var AppConfig *Config

func Load() error {
	if err := godotenv.Load(); err != nil {
		// No .env file, use defaults or env vars
	}

	sshTimeout, _ := strconv.Atoi(getEnv("SSH_TIMEOUT", "30"))
	sshKeepAlive, _ := strconv.Atoi(getEnv("SSH_KEEPALIVE", "60"))
	metricsInterval, _ := strconv.Atoi(getEnv("METRICS_INTERVAL", "10"))
	wsPingInterval, _ := strconv.Atoi(getEnv("WS_PING_INTERVAL", "30"))
	wsPongWait, _ := strconv.Atoi(getEnv("WS_PONG_WAIT", "60"))

	AppConfig = &Config{
		ServerPort:      getEnv("SERVER_PORT", "8080"),
		DBHost:          getEnv("DB_HOST", "localhost"),
		DBPort:          getEnv("DB_PORT", "3306"),
		DBUser:          getEnv("DB_USER", "root"),
		DBPassword:      getEnv("DB_PASSWORD", ""),
		DBName:          getEnv("DB_NAME", "Suap"),
		SSHTimeout:      time.Duration(sshTimeout) * time.Second,
		SSHKeepAlive:    time.Duration(sshKeepAlive) * time.Second,
		MetricsInterval: time.Duration(metricsInterval) * time.Second,
		EncryptionKey:   getEnv("ENCRYPTION_KEY", "3nC_rYpT!8t2vKp#6Lq1zWm9x4Dg7HsQ"),
		WSPingInterval:  time.Duration(wsPingInterval) * time.Second,
		WSPongWait:      time.Duration(wsPongWait) * time.Second,
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
