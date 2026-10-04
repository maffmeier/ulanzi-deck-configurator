package deck

// Physical layout of the Ulanzi D200: 13 square buttons on a 5x3 grid plus
// a wide info window occupying the last two cells of the bottom row.
const (
	VendorID  = 0x2207
	ProductID = 0x0019

	ButtonCount     = 13
	InfoWindowIndex = 13
	SlotCount       = 14
	GridColumns     = 5

	IconSize             = 196
	InfoWindowWidth      = IconSize * 2
	InfoWindowHeight     = IconSize
	DefaultBrightness    = 50
	DefaultPageName      = "default"
	EditorDefaultPage    = "main"
	DefaultTimeFormat    = "%H:%M"
	DefaultTextBg        = "#111827"
	DefaultTextColor     = "#F8FAFC"
	DefaultFontFamily    = "DejaVu Sans"
	DefaultFontSize      = 30
	DefaultSmallWindowBg = "#000000"
)

// Small-window refresh bounds: the firmware watchdog fires after ~5s, the
// lower bound only prevents a busy loop on the USB bus.
const (
	SmallWindowMinInterval = 0.05
	SmallWindowMaxInterval = 4.5
	MaxTemperatureSensors  = 3
)

var MetricChoices = []string{"cpu", "memory", "gpu", "temperature", "disk", "network", "battery"}

var MetricLabels = map[string]string{
	"cpu":         "CPU",
	"memory":      "MEM",
	"gpu":         "GPU",
	"temperature": "TEMP",
	"disk":        "DISK",
	"network":     "NET",
	"battery":     "BAT",
}

var FontFamilies = []string{
	"DejaVu Sans",
	"DejaVu Serif",
	"DejaVu Sans Mono",
	"Liberation Sans",
	"Liberation Serif",
}
