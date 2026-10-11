package application

import "time"

// AudioInfo is the audio state as widgets show it.
type AudioInfo struct {
	Volume   int
	Muted    bool
	MicMuted bool
	Output   string
	AppFound bool
}

// AudioSource reads the host audio state; it is optional for the engine.
type AudioSource interface {
	AudioState(target string) (AudioInfo, error)
}

// pollKeyed returns the cached value for key and refreshes it in the
// background once it is older than every. The caller must not hold e.mu.
func pollKeyed[T any](e *WidgetEngine, cache map[string]*polled[T], key string, every time.Duration, now time.Time, fetch func() (T, error)) (T, error, time.Time) {
	e.mu.Lock()
	p := cache[key]
	if p == nil {
		p = &polled[T]{}
		cache[key] = p
	}
	if !p.running && now.Sub(p.at) >= every {
		p.running = true
		go func() {
			v, err := fetch()
			e.mu.Lock()
			defer e.mu.Unlock()
			if cur := cache[key]; cur != nil {
				cur.value, cur.err, cur.at, cur.running = v, err, time.Now(), false
			}
		}()
	}
	v, err, at := p.value, p.err, p.at
	e.mu.Unlock()
	return v, err, at
}

// expire makes the next render of every cached entry fetch again, e.g.
// right after an action changed the state.
func expire[T any](e *WidgetEngine, cache map[string]*polled[T]) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range cache {
		p.at = time.Time{}
	}
}
