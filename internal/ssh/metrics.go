package ssh

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"monitoring/internal/models"
	"monitoring/internal/utils"
)

// MetricCollector collects system metrics via SSH
type MetricCollector struct {
	client *SSHClient
	logger *utils.ContextLogger
}

// NewMetricCollector creates a new metric collector
func NewMetricCollector(client *SSHClient) *MetricCollector {
	return &MetricCollector{
		client: client,
		logger: utils.AppLogger.WithContext(client.Server.ID, client.Server.Name),
	}
}

// CollectAll collects all metrics from the server
func (m *MetricCollector) CollectAll() (*models.MetricSnapshot, error) {
	snapshot := &models.MetricSnapshot{
		ServerID:   m.client.Server.ID,
		ServerName: m.client.Server.Name,
		Timestamp:  time.Now().Unix(),
	}

	// Collect CPU usage
	cpu, err := m.CollectCPU()
	if err != nil {
		m.logger.Warning("Failed to collect CPU: %v", err)
	} else {
		snapshot.CPUUsage = cpu
	}

	// Collect memory
	memTotal, memUsed, memFree, err := m.CollectMemory()
	if err != nil {
		m.logger.Warning("Failed to collect memory: %v", err)
	} else {
		snapshot.MemTotal = memTotal
		snapshot.MemUsed = memUsed
		snapshot.MemFree = memFree
		if memTotal > 0 {
			snapshot.MemPercent = float64(memUsed) / float64(memTotal) * 100
		}
	}

	// Collect disk
	diskTotal, diskUsed, diskFree, err := m.CollectDisk()
	if err != nil {
		m.logger.Warning("Failed to collect disk: %v", err)
	} else {
		snapshot.DiskTotal = diskTotal
		snapshot.DiskUsed = diskUsed
		snapshot.DiskFree = diskFree
		if diskTotal > 0 {
			snapshot.DiskPercent = float64(diskUsed) / float64(diskTotal) * 100
		}
	}

	// Collect network
	rx, tx, err := m.CollectNetwork()
	if err != nil {
		m.logger.Warning("Failed to collect network: %v", err)
	} else {
		snapshot.NetRX = rx
		snapshot.NetTX = tx
	}

	// Collect uptime
	uptime, err := m.CollectUptime()
	if err != nil {
		m.logger.Warning("Failed to collect uptime: %v", err)
	} else {
		snapshot.Uptime = uptime
	}

	return snapshot, nil
}

func (m *MetricCollector) CollectCPU() (float64, error) {

	cmd := `top -bn2 -d0.5 | grep "Cpu(s)" | tail -1 | awk '{print $2}' | cut -d'%' -f1`
	output, err := m.client.Execute(cmd)
	if err != nil {
		// Fallback method using /proc/stat
		return m.collectCPUFromProc()
	}

	cpu, err := strconv.ParseFloat(strings.TrimSpace(output), 64)
	if err != nil {
		return m.collectCPUFromProc()
	}

	return cpu, nil
}

func (m *MetricCollector) collectCPUFromProc() (float64, error) {
	// Get two readings 1 second apart
	cmd := `cat /proc/stat | grep '^cpu ' | awk '{print $2+$3+$4, $5}' && sleep 1 && cat /proc/stat | grep '^cpu ' | awk '{print $2+$3+$4, $5}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return 0, nil
	}

	// Parse first reading
	parts1 := strings.Fields(lines[0])
	if len(parts1) < 2 {
		return 0, nil
	}
	active1, _ := strconv.ParseFloat(parts1[0], 64)
	idle1, _ := strconv.ParseFloat(parts1[1], 64)

	// Parse second reading
	parts2 := strings.Fields(lines[1])
	if len(parts2) < 2 {
		return 0, nil
	}
	active2, _ := strconv.ParseFloat(parts2[0], 64)
	idle2, _ := strconv.ParseFloat(parts2[1], 64)

	// Calculate CPU percentage
	activeDiff := active2 - active1
	idleDiff := idle2 - idle1
	total := activeDiff + idleDiff

	if total == 0 {
		return 0, nil
	}

	return (activeDiff / total) * 100, nil
}

// CollectMemory collects memory usage in MB
func (m *MetricCollector) CollectMemory() (total, used, free uint64, err error) {
	cmd := `free -m | grep Mem | awk '{print $2, $3, $4}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 3 {
		return 0, 0, 0, nil
	}

	total, _ = strconv.ParseUint(parts[0], 10, 64)
	used, _ = strconv.ParseUint(parts[1], 10, 64)
	free, _ = strconv.ParseUint(parts[2], 10, 64)

	return total, used, free, nil
}

// CollectDisk collects disk usage in GB (root partition)
func (m *MetricCollector) CollectDisk() (total, used, free uint64, err error) {
	cmd := `df -BG / | tail -1 | awk '{gsub("G",""); print $2, $3, $4}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 3 {
		return 0, 0, 0, nil
	}

	total, _ = strconv.ParseUint(parts[0], 10, 64)
	used, _ = strconv.ParseUint(parts[1], 10, 64)
	free, _ = strconv.ParseUint(parts[2], 10, 64)

	return total, used, free, nil
}

// CollectNetwork collects network traffic in MB
func (m *MetricCollector) CollectNetwork() (rx, tx uint64, err error) {
	// Get the primary interface and its traffic
	cmd := `cat /proc/net/dev | grep -E '(eth0|ens|enp)' | head -1 | awk '{print $2, $10}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 2 {
		// Try alternative approach
		cmd = `ip -s link show | grep -A1 'RX:' | tail -1 | awk '{print $1}' && ip -s link show | grep -A1 'TX:' | tail -1 | awk '{print $1}'`
		output, err = m.client.Execute(cmd)
		if err != nil {
			return 0, 0, err
		}
		parts = strings.Fields(strings.TrimSpace(output))
		if len(parts) < 2 {
			return 0, 0, nil
		}
	}

	rxBytes, _ := strconv.ParseUint(parts[0], 10, 64)
	txBytes, _ := strconv.ParseUint(parts[1], 10, 64)

	// Convert bytes to MB
	rx = rxBytes / (1024 * 1024)
	tx = txBytes / (1024 * 1024)

	return rx, tx, nil
}

// CollectUptime collects system uptime in seconds
func (m *MetricCollector) CollectUptime() (uint64, error) {
	cmd := `cat /proc/uptime | awk '{print int($1)}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, err
	}

	uptime, err := strconv.ParseUint(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return 0, err
	}

	return uptime, nil
}

// CollectProcesses collects running processes count
func (m *MetricCollector) CollectProcesses() (int, error) {
	cmd := `ps aux | wc -l`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		return 0, err
	}

	return count - 1, nil // Subtract header line
}

// CollectLoadAverage collects system load average
func (m *MetricCollector) CollectLoadAverage() (load1, load5, load15 float64, err error) {
	cmd := `cat /proc/loadavg | awk '{print $1, $2, $3}'`
	output, err := m.client.Execute(cmd)
	if err != nil {
		return 0, 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(output))
	if len(parts) < 3 {
		return 0, 0, 0, nil
	}

	load1, _ = strconv.ParseFloat(parts[0], 64)
	load5, _ = strconv.ParseFloat(parts[1], 64)
	load15, _ = strconv.ParseFloat(parts[2], 64)

	return load1, load5, load15, nil
}

// CollectHostname collects the server hostname
func (m *MetricCollector) CollectHostname() (string, error) {
	output, err := m.client.Execute("hostname")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// CollectOSInfo collects OS information
func (m *MetricCollector) CollectOSInfo() (string, error) {
	output, err := m.client.Execute("cat /etc/os-release | grep PRETTY_NAME | cut -d'\"' -f2")
	if err != nil {
		// Fallback
		output, err = m.client.Execute("uname -a")
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(output), nil
}

// CollectTopProcesses collects top CPU consuming processes
func (m *MetricCollector) CollectTopProcesses(limit int) ([]map[string]string, error) {
	cmd := `ps aux --sort=-%cpu | head -` + strconv.Itoa(limit+1) + ` | tail -` + strconv.Itoa(limit)
	output, err := m.client.Execute(cmd)
	if err != nil {
		return nil, err
	}

	var processes []map[string]string
	lines := strings.Split(strings.TrimSpace(output), "\n")
	re := regexp.MustCompile(`\s+`)

	for _, line := range lines {
		parts := re.Split(line, 11)
		if len(parts) >= 11 {
			processes = append(processes, map[string]string{
				"user":    parts[0],
				"pid":     parts[1],
				"cpu":     parts[2],
				"mem":     parts[3],
				"command": parts[10],
			})
		}
	}

	return processes, nil
}
