package deck

import (
	"errors"
	"time"
)

var ErrNotConnected = errors.New("deck not connected")

type Event interface{ isEvent() }

type ButtonEvent struct {
	Index      int
	Pressed    bool
	State      int
	OccurredAt time.Time
}

type DeviceInfoEvent struct {
	Info       string
	OccurredAt time.Time
}

type ConnectionEvent struct {
	Connected bool
	Err       error
}

func (ButtonEvent) isEvent()     {}
func (DeviceInfoEvent) isEvent() {}
func (ConnectionEvent) isEvent() {}

type SmallWindowMode int

const (
	SmallWindowStats      SmallWindowMode = 0
	SmallWindowClock      SmallWindowMode = 1
	SmallWindowBackground SmallWindowMode = 2
)

func (m SmallWindowMode) String() string {
	switch m {
	case SmallWindowStats:
		return "STATS"
	case SmallWindowClock:
		return "CLOCK"
	case SmallWindowBackground:
		return "BACKGROUND"
	}
	return "UNKNOWN"
}
