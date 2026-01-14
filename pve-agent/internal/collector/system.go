package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/moconnor/pve-agent/internal/types"
)

// GetSystemMetrics reads metrics from /proc filesystem
func (c *PVEClient) GetSystemMetrics() (*types.SystemMetrics, error) {
	metrics := &types.SystemMetrics{}

	// Read /proc/vmstat for memory I/O metrics
	if err := readVMStat(metrics); err != nil {
		return nil, err
	}

	// Read /proc/loadavg for load averages
	if err := readLoadAvg(metrics); err != nil {
		return nil, err
	}

	return metrics, nil
}

func readVMStat(m *types.SystemMetrics) error {
	file, err := os.Open("/proc/vmstat")
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}

		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "pgpgin":
			m.PgpgIn = val
		case "pgpgout":
			m.PgpgOut = val
		case "pswpin":
			m.PswpIn = val
		case "pswpout":
			m.PswpOut = val
		case "pgfault":
			m.PgFault = val
		case "pgmajfault":
			m.PgMajFault = val
		}
	}

	return scanner.Err()
}

func readLoadAvg(m *types.SystemMetrics) error {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return err
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil
	}

	if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
		m.LoadAvg1 = v
	}
	if v, err := strconv.ParseFloat(fields[1], 64); err == nil {
		m.LoadAvg5 = v
	}
	if v, err := strconv.ParseFloat(fields[2], 64); err == nil {
		m.LoadAvg15 = v
	}

	return nil
}
