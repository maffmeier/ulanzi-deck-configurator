package actions

import (
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

func TestEveryMeetingOpHasAParsableShortcut(t *testing.T) {
	for app, ops := range deck.MeetingOps {
		for _, op := range ops {
			for i, keys := range meetingKeys[app][op] {
				if keys == "" {
					t.Fatalf("%s %s: missing shortcut %d", app, op, i)
				}
				if _, err := ParseCombo(keys); err != nil {
					t.Fatalf("%s %s %q: %v", app, op, keys, err)
				}
			}
			if _, err := MeetingShortcut(app, op); err != nil {
				t.Fatal(err)
			}
		}
	}
}
