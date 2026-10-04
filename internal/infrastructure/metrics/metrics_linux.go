package metrics

import (
	"math"
	"path/filepath"
	"strconv"
)

// Linux sensor ids are thermal zone names (thermal_zone0, ...) to stay
// compatible with configs written by ulanzi-linux.
func temperatureSensors() []TemperatureSensor {
	var sensors []TemperatureSensor
	for _, tempFile := range globSorted("/sys/class/thermal/thermal_zone*/temp") {
		zone := filepath.Dir(tempFile)
		id := filepath.Base(zone)
		name, err := readTrimmed(filepath.Join(zone, "type"))
		if err != nil || name == "" {
			name = id
		}
		sensors = append(sensors, TemperatureSensor{ID: id, Name: name, ValueCelsius: parseMilliCelsius(tempFile)})
	}
	return sensors
}

func parseMilliCelsius(path string) *int {
	raw, err := readTrimmed(path)
	if err != nil {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	switch {
	case value >= 1000 && value <= 200000:
		v := int(math.Round(float64(value) / 1000))
		return &v
	case value >= 1 && value <= 200:
		return &value
	}
	return nil
}

func batteryPercent() (int, bool) {
	for _, path := range globSorted("/sys/class/power_supply/BAT*/capacity") {
		raw, err := readTrimmed(path)
		if err != nil {
			continue
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return clampPercent(v), true
		}
	}
	return 0, false
}

func gpuPercent() (int, bool) {
	for _, path := range globSorted("/sys/class/drm/card*/device/gpu_busy_percent") {
		raw, err := readTrimmed(path)
		if err != nil {
			continue
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return clampPercent(v), true
		}
	}
	for _, path := range globSorted("/sys/class/drm/card*/device/load") {
		raw, err := readTrimmed(path)
		if err != nil {
			continue
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			if v > 100 {
				v /= 10
			}
			return clampPercent(v), true
		}
	}
	return 0, false
}
