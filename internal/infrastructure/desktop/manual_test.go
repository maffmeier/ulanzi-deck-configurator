package desktop

import (
	"os"
	"testing"
)

// Prints what the desktop reports; run with DESKTOP_MANUAL=1.
func TestManualProbe(t *testing.T) {
	if os.Getenv("DESKTOP_MANUAL") == "" {
		t.Skip("DESKTOP_MANUAL not set")
	}
	app, err := ActiveApp()
	t.Logf("active app: %q (err %v)", app, err)
	locked, err := Locked()
	t.Logf("locked: %v (err %v)", locked, err)
}
