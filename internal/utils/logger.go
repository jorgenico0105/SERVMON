package utils

import (
	"fmt"
	"log"
	"os"
	"time"
)

type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarning
	LogError
)

type Logger struct {
	debugLogger   *log.Logger
	infoLogger    *log.Logger
	warningLogger *log.Logger
	errorLogger   *log.Logger
	minLevel      LogLevel
}

var AppLogger *Logger

func InitLogger(minLevel LogLevel) {
	AppLogger = &Logger{
		debugLogger:   log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile),
		infoLogger:    log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile),
		warningLogger: log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile),
		errorLogger:   log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile),
		minLevel:      minLevel,
	}
}

func (l *Logger) Debug(format string, v ...interface{}) {
	if l.minLevel <= LogDebug {
		l.debugLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	if l.minLevel <= LogInfo {
		l.infoLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Warning(format string, v ...interface{}) {
	if l.minLevel <= LogWarning {
		l.warningLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	if l.minLevel <= LogError {
		l.errorLogger.Output(2, fmt.Sprintf(format, v...))
	}
}

// Structured logging with context
func (l *Logger) WithContext(serverID uint, serverName string) *ContextLogger {
	return &ContextLogger{
		logger:     l,
		serverID:   serverID,
		serverName: serverName,
	}
}

type ContextLogger struct {
	logger     *Logger
	serverID   uint
	serverName string
}

func (c *ContextLogger) prefix() string {
	return fmt.Sprintf("[Server:%d:%s] ", c.serverID, c.serverName)
}

func (c *ContextLogger) Debug(format string, v ...interface{}) {
	c.logger.Debug(c.prefix()+format, v...)
}

func (c *ContextLogger) Info(format string, v ...interface{}) {
	c.logger.Info(c.prefix()+format, v...)
}

func (c *ContextLogger) Warning(format string, v ...interface{}) {
	c.logger.Warning(c.prefix()+format, v...)
}

func (c *ContextLogger) Error(format string, v ...interface{}) {
	c.logger.Error(c.prefix()+format, v...)
}

// FormatUptime converts seconds to human readable format
func FormatUptime(seconds uint64) string {
	duration := time.Duration(seconds) * time.Second
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// GenerateID generates a unique ID
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ParseDuration converts seconds to time.Duration
func ParseDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

// FormatPercent formats a float as percentage string
func FormatPercent(v float64) string {
	return fmt.Sprintf("%.2f%%", v)
}
