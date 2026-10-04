package metrics

import (
	"syscall"
	"unsafe"
)

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var procGetSystemPowerStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

func batteryPercent() (int, bool) {
	var status systemPowerStatus
	ok, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	// 128 = no system battery, 255 = unknown percentage.
	if ok == 0 || status.BatteryFlag == 128 || status.BatteryLifePercent == 255 {
		return 0, false
	}
	return int(status.BatteryLifePercent), true
}
