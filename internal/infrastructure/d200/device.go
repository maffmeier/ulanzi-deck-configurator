package d200

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// Transport is one opened HID handle; reads block until a report arrives.
type Transport interface {
	ReadReport() ([]byte, error)
	WriteReport(frame []byte) error
	Close() error
}

type Opener func() (Transport, error)

const reconnectInterval = time.Second

// labelStyle is sent once before the first button upload; the key order is
// the one the official client uses.
const labelStyle = `{"Align":"bottom","Color":16777215,"FontName":"Roboto","ShowTitle":true,"Size":10,"Weight":80}`

type smallWindowData struct {
	cpu, mem, gpu int
	time          string
}

// Device talks to one D200. Every setter updates a cached copy of the
// device state first, so commands issued while the deck is unplugged are
// replayed as soon as it reconnects.
type Device struct {
	open   Opener
	log    *slog.Logger
	events chan deck.Event

	mu                sync.Mutex
	t                 Transport
	brightness        *int
	swMode            *deck.SmallWindowMode
	swData            *smallWindowData
	labelStyleApplied bool
	buttons           map[int]deck.Button
	buttonsFull       bool
}

func NewDevice(open Opener, log *slog.Logger) *Device {
	return &Device{
		open:    open,
		log:     log,
		events:  make(chan deck.Event, 256),
		buttons: map[int]deck.Button{},
	}
}

func (d *Device) Events() <-chan deck.Event { return d.events }

func (d *Device) Connected() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.t != nil
}

// Run keeps the deck connected until ctx ends, reopening it after unplugs.
func (d *Device) Run(ctx context.Context) {
	waitingLogged := false
	for ctx.Err() == nil {
		t, err := d.open()
		if err != nil {
			if !waitingLogged {
				d.log.Info("waiting for deck", "error", err)
				waitingLogged = true
			}
			sleep(ctx, reconnectInterval)
			continue
		}
		waitingLogged = false

		if err := d.attach(t); err != nil {
			d.log.Warn("deck setup failed", "error", err)
			sleep(ctx, reconnectInterval)
			continue
		}
		d.log.Info("deck connected")
		d.emit(deck.ConnectionEvent{Connected: true})

		stop := context.AfterFunc(ctx, func() { d.detach(t) })
		err = d.readLoop(t)
		stop()
		d.detach(t)
		if ctx.Err() == nil {
			d.log.Warn("deck disconnected", "error", err)
			d.emit(deck.ConnectionEvent{Connected: false, Err: err})
			sleep(ctx, reconnectInterval)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (d *Device) attach(t Transport) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.restore(t); err != nil {
		t.Close()
		return err
	}
	d.t = t
	return nil
}

func (d *Device) detach(t Transport) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.t == t {
		d.t = nil
	}
	t.Close()
}

func (d *Device) readLoop(t Transport) error {
	for {
		raw, err := t.ReadReport()
		if err != nil {
			return err
		}
		pkt, ok := parseIncoming(raw)
		if !ok {
			continue
		}
		switch pkt.command {
		case inButton:
			if len(pkt.data) < 4 {
				continue
			}
			ev := deck.ButtonEvent{
				State:      int(pkt.data[0]),
				Index:      int(pkt.data[1]),
				Pressed:    pkt.data[3] == 0x01,
				OccurredAt: time.Now(),
			}
			d.log.Debug("button event", "index", ev.Index, "pressed", ev.Pressed)
			d.emit(ev)
		case inDeviceInfo:
			info := cString(pkt.data)
			d.log.Info("device info", "info", info)
			d.emit(deck.DeviceInfoEvent{Info: info, OccurredAt: time.Now()})
		}
	}
}

// emit drops the oldest event when nobody keeps up, so the read loop never
// stalls and the latest input always wins.
func (d *Device) emit(ev deck.Event) {
	for {
		select {
		case d.events <- ev:
			return
		default:
			select {
			case <-d.events:
				d.log.Warn("event queue overflow, dropped oldest")
			default:
			}
		}
	}
}

// writeLocked sends frames on the current transport; a failed write drops
// the transport so Run reconnects and restores the cached state.
func (d *Device) writeLocked(frames ...[]byte) error {
	if d.t == nil {
		return deck.ErrNotConnected
	}
	for _, frame := range frames {
		if err := d.t.WriteReport(frame); err != nil {
			t := d.t
			d.t = nil
			t.Close()
			return fmt.Errorf("write failed: %w", err)
		}
	}
	return nil
}

func (d *Device) SetBrightness(value int, force bool) error {
	if value < 0 || value > 100 {
		return fmt.Errorf("brightness must be in 0..100, got %d", value)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !force && d.brightness != nil && *d.brightness == value {
		return nil
	}
	d.brightness = &value
	return d.writeLocked(brightnessFrame(value))
}

func brightnessFrame(value int) []byte {
	// The firmware expects ASCII digits, not a raw byte.
	payload := []byte(strconv.Itoa(value))
	return Frame(CmdSetBrightness, len(payload), payload)
}

func (d *Device) SetSmallWindowMode(mode deck.SmallWindowMode) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.swMode = &mode
	// CLOCK/STATS are applied with the next data packet; only BACKGROUND
	// needs an explicit mode switch.
	if mode != deck.SmallWindowBackground {
		return nil
	}
	return d.writeLocked(smallWindowFrame(mode, smallWindowData{time: "00:00:00"}))
}

func (d *Device) SetSmallWindowData(cpu, mem, gpu int, timeStr string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	data := smallWindowData{cpu, mem, gpu, timeStr}
	d.swData = &data
	return d.writeLocked(smallWindowFrame(d.activeModeLocked(), data))
}

func (d *Device) activeModeLocked() deck.SmallWindowMode {
	if d.swMode == nil {
		return deck.SmallWindowClock
	}
	return *d.swMode
}

func smallWindowFrame(mode deck.SmallWindowMode, data smallWindowData) []byte {
	payload := fmt.Appendf(nil, "%d|%d|%d|%s|%d", int(mode), data.cpu, data.mem, data.time, data.gpu)
	return Frame(CmdSetSmallWindowData, len(payload), payload)
}

// KeepAlive feeds the firmware watchdog (~5s); without traffic the deck
// falls back to its standalone screen.
func (d *Device) KeepAlive() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeLocked(d.keepAliveFrameLocked())
}

func (d *Device) keepAliveFrameLocked() []byte {
	mode := d.activeModeLocked()
	if mode == deck.SmallWindowBackground {
		return smallWindowFrame(mode, smallWindowData{time: "00:00:00"})
	}
	if d.swData != nil {
		return smallWindowFrame(mode, *d.swData)
	}
	return Frame(CmdSetSmallWindowData, 0, nil)
}

// SetButtons uploads a layout. A full upload replaces the 13 grid buttons
// (missing ones turn black); a partial upload only touches the given ones.
func (d *Device) SetButtons(buttons []deck.Button, partial bool) error {
	blob, err := BuildButtonsZip(buttons, !partial)
	if err != nil {
		return err
	}
	cmd := CmdSetButtons
	if partial {
		cmd = CmdPartialButtons
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !partial {
		clear(d.buttons)
		d.buttonsFull = true
	}
	for _, b := range buttons {
		d.buttons[b.Index] = b
	}

	frames := Chunks(cmd, blob)
	if !d.labelStyleApplied {
		if err := d.writeLocked(Frame(CmdSetLabelStyle, len(labelStyle), []byte(labelStyle))); err != nil {
			return err
		}
		d.labelStyleApplied = true
	}
	d.log.Debug("uploading buttons", "command", cmd, "bytes", len(blob), "frames", len(frames))
	return d.writeLocked(frames...)
}

// restore replays the cached state onto a freshly opened transport.
func (d *Device) restore(t Transport) error {
	write := func(frames ...[]byte) error {
		for _, f := range frames {
			if err := t.WriteReport(f); err != nil {
				return err
			}
		}
		return nil
	}

	if d.brightness != nil {
		if err := write(brightnessFrame(*d.brightness)); err != nil {
			return err
		}
	}
	if d.labelStyleApplied || len(d.buttons) > 0 {
		if err := write(Frame(CmdSetLabelStyle, len(labelStyle), []byte(labelStyle))); err != nil {
			return err
		}
		d.labelStyleApplied = true
	}

	var grid, extra []deck.Button
	for _, idx := range slices.Sorted(maps.Keys(d.buttons)) {
		if idx < deck.ButtonCount {
			grid = append(grid, d.buttons[idx])
		} else {
			extra = append(extra, d.buttons[idx])
		}
	}
	if len(grid) > 0 {
		cmd := CmdPartialButtons
		if d.buttonsFull {
			cmd = CmdSetButtons
		}
		blob, err := BuildButtonsZip(grid, d.buttonsFull)
		if err != nil {
			return err
		}
		if err := write(Chunks(cmd, blob)...); err != nil {
			return err
		}
	}
	if len(extra) > 0 {
		blob, err := BuildButtonsZip(extra, false)
		if err != nil {
			return err
		}
		if err := write(Chunks(CmdPartialButtons, blob)...); err != nil {
			return err
		}
	}

	if d.swMode != nil || d.swData != nil {
		return write(d.keepAliveFrameLocked())
	}
	return nil
}
