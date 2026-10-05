package media

import "testing"

// Runs on the Windows CI runner: proves the WinRT activation, the async
// polling and the session query work. A runner has no media session, so
// an empty result is the expected outcome.
func TestCurrentDoesNotFail(t *testing.T) {
	info, err := Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	t.Logf("now playing: %+v", info)
	// A second call must reuse the cached session manager.
	if _, err := Current(); err != nil {
		t.Fatalf("second Current: %v", err)
	}
}
