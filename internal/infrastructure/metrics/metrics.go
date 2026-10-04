// Package metrics reads host statistics for the info window.
package metrics

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/strftime"
)

type TemperatureSensor struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ValueCelsius *int   `json:"value_celsius"`
}

// Reader is stateful: CPU load and network rate are deltas between calls,
// so the first call after start returns 0.
type Reader struct {
	mu      sync.Mutex
	lastCPU *cpu.TimesStat
	lastNet *netSample
}

type netSample struct {
	bytes uint64
	at    time.Time
}

func NewReader() *Reader { return &Reader{} }

func (r *Reader) FormatTime(format string) string {
	return strftime.Format(format, time.Now())
}

func (r *Reader) CPUPercent() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0
	}
	current := times[0]
	previous := r.lastCPU
	r.lastCPU = &current
	if previous == nil {
		return 0
	}
	busy := func(t *cpu.TimesStat) float64 {
		return t.User + t.Nice + t.System + t.Irq + t.Softirq + t.Steal
	}
	total := func(t *cpu.TimesStat) float64 { return busy(t) + t.Idle + t.Iowait }
	totalDelta := total(&current) - total(previous)
	if totalDelta <= 0 {
		return 0
	}
	return clampPercent((busy(&current) - busy(previous)) / totalDelta * 100)
}

func (r *Reader) MemoryPercent() int {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0
	}
	return clampPercent(vm.UsedPercent)
}

func (r *Reader) DiskPercent() int {
	usage, err := disk.Usage(rootPath())
	if err != nil {
		return 0
	}
	return clampPercent(usage.UsedPercent)
}

func rootPath() string {
	if runtime.GOOS == "windows" {
		drive := os.Getenv("SystemDrive")
		if drive == "" {
			drive = "C:"
		}
		return drive + `\`
	}
	return "/"
}

func (r *Reader) NetworkRate() string {
	counters, err := net.IOCounters(true)
	if err != nil {
		return "n/a"
	}
	var total uint64
	for _, c := range counters {
		name := strings.ToLower(c.Name)
		if name == "lo" || strings.HasPrefix(name, "loopback") || name == "lo0" {
			continue
		}
		total += c.BytesRecv + c.BytesSent
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	previous := r.lastNet
	r.lastNet = &netSample{total, now}
	if previous == nil || total < previous.bytes {
		return "0 B/s"
	}
	elapsed := math.Max(0.001, now.Sub(previous.at).Seconds())
	return formatBytes(float64(total-previous.bytes)/elapsed) + "/s"
}

// MetricValue renders one metric for the info window.
func (r *Reader) MetricValue(metric string) string {
	switch metric {
	case "cpu":
		return fmt.Sprintf("%d%%", r.CPUPercent())
	case "memory":
		return fmt.Sprintf("%d%%", r.MemoryPercent())
	case "gpu":
		if v, ok := gpuPercent(); ok {
			return fmt.Sprintf("%d%%", v)
		}
		return "n/a"
	case "temperature":
		return r.TemperatureValue(nil, " ")
	case "disk":
		return fmt.Sprintf("%d%%", r.DiskPercent())
	case "network":
		return r.NetworkRate()
	case "battery":
		if v, ok := batteryPercent(); ok {
			return fmt.Sprintf("%d%%", v)
		}
		return "n/a"
	}
	return "n/a"
}

func (r *Reader) TemperatureSensors() []TemperatureSensor {
	return temperatureSensors()
}

// TemperatureValue joins the selected sensors in order; without a
// selection the first sensor with a valid reading is used.
func (r *Reader) TemperatureValue(sensorIDs []string, separator string) string {
	sensors := temperatureSensors()
	format := func(s *TemperatureSensor) string {
		if s == nil || s.ValueCelsius == nil {
			return "n/a"
		}
		return fmt.Sprintf("%dC", *s.ValueCelsius)
	}
	if len(sensorIDs) == 0 {
		for i := range sensors {
			if sensors[i].ValueCelsius != nil {
				return format(&sensors[i])
			}
		}
		return "n/a"
	}
	values := make([]string, 0, len(sensorIDs))
	for _, id := range sensorIDs {
		var found *TemperatureSensor
		for i := range sensors {
			if sensors[i].ID == id {
				found = &sensors[i]
				break
			}
		}
		values = append(values, format(found))
	}
	joiner := " "
	if separator == "|" {
		joiner = " | "
	}
	return strings.Join(values, joiner)
}

func clampPercent(v float64) int {
	return int(math.Max(0, math.Min(100, math.Round(v))))
}

func formatBytes(v float64) string {
	units := []string{"B", "K", "M", "G"}
	for i, unit := range units {
		if v < 1024 || i == len(units)-1 {
			if unit == "B" {
				return fmt.Sprintf("%.0f %s", v, unit)
			}
			return strings.Replace(fmt.Sprintf("%.1f %s", v, unit), ".0 ", " ", 1)
		}
		v /= 1024
	}
	return ""
}

func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func globSorted(pattern string) []string {
	matches, _ := filepath.Glob(pattern)
	return matches
}
