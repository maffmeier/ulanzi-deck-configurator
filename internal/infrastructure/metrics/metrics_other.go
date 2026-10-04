//go:build !linux

package metrics

import (
	"math"

	"github.com/shirou/gopsutil/v4/sensors"
)

func temperatureSensors() []TemperatureSensor {
	stats, err := sensors.SensorsTemperatures()
	if err != nil && len(stats) == 0 {
		return nil
	}
	result := make([]TemperatureSensor, 0, len(stats))
	for _, s := range stats {
		var value *int
		if s.Temperature > 0 && s.Temperature < 200 {
			v := int(math.Round(s.Temperature))
			value = &v
		}
		result = append(result, TemperatureSensor{ID: s.SensorKey, Name: s.SensorKey, ValueCelsius: value})
	}
	return result
}

// GPU load has no portable source outside Linux sysfs.
func gpuPercent() (int, bool) { return 0, false }
