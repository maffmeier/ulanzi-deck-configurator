package audio

import (
	"os"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// AUDIO_MANUAL=1 runs against the real audio system and restores it:
// mic mute twice, volume up and down again.
func TestManualAudio(t *testing.T) {
	if os.Getenv("AUDIO_MANUAL") == "" {
		t.Skip("set AUDIO_MANUAL=1 to touch the real audio system")
	}
	before, err := Current("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: %+v", before)
	for _, op := range []string{deck.AudioMicMute, deck.AudioMicMute, deck.AudioVolumeUp, deck.AudioVolumeDown} {
		if err := Run(deck.Action{Type: deck.ActionAudio, Op: op}); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		st, _ := Current("")
		t.Logf("after %s: %+v", op, st)
	}
	after, _ := Current("")
	if after.Volume != before.Volume || after.MicMuted != before.MicMuted {
		t.Fatalf("state not restored: %+v -> %+v", before, after)
	}
	if _, err := Current("no-such-app"); err != nil {
		t.Fatal(err)
	}
}
