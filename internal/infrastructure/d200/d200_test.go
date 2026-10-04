package d200

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

func TestFrameLayout(t *testing.T) {
	frame := Frame(CmdSetBrightness, 2, []byte("50"))
	if len(frame) != PacketSize {
		t.Fatalf("frame size %d", len(frame))
	}
	want := []byte{0x7C, 0x7C, 0x00, 0x0A, 0x02, 0x00, 0x00, 0x00, '5', '0', 0x00}
	if !bytes.Equal(frame[:len(want)], want) {
		t.Fatalf("header % x", frame[:len(want)])
	}
}

func TestChunksCarryTotalLengthAndRawContinuations(t *testing.T) {
	blob := bytes.Repeat([]byte{0xAB}, 3000)
	frames := Chunks(CmdSetButtons, blob)
	if len(frames) != 3 {
		t.Fatalf("got %d frames", len(frames))
	}
	if frames[0][4] != 0xB8 || frames[0][5] != 0x0B {
		t.Fatalf("length field % x", frames[0][4:8])
	}
	if frames[1][0] != 0xAB {
		t.Fatal("continuation frame must not carry a header")
	}
	var joined []byte
	joined = append(joined, frames[0][HeaderSize:]...)
	for _, f := range frames[1:] {
		joined = append(joined, f...)
	}
	if !bytes.Equal(joined[:len(blob)], blob) {
		t.Fatal("payload not preserved")
	}
}

func readZip(t *testing.T, blob []byte) (map[string][]byte, []string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var order []string
	for _, f := range zr.File {
		if f.Flags&0x8 != 0 {
			t.Fatalf("%s uses a data descriptor", f.Name)
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = data
		order = append(order, f.Name)
	}
	return files, order
}

func TestButtonsZip(t *testing.T) {
	buttons := []deck.Button{
		{Index: 0, Label: "Hallo", TextStyle: deck.DefaultTextStyle()},
		{Index: 6, Label: "Grüße", TextStyle: deck.DefaultTextStyle()},
	}
	blob, err := BuildButtonsZip(buttons, true)
	if err != nil {
		t.Fatal(err)
	}
	if !boundariesSafe(blob) {
		t.Fatal("frame boundary bytes not safe")
	}
	files, order := readZip(t, blob)
	if order[0] != "manifest.json" || order[1] != "dummy.txt" || order[len(order)-1] != "sentinel.txt" {
		t.Fatalf("entry order %v", order)
	}

	raw := files["manifest.json"]
	if bytes.ContainsRune(raw, 'ü') {
		t.Fatal("manifest must be ASCII-escaped")
	}
	var manifest map[string]manifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != deck.ButtonCount {
		t.Fatalf("full upload must fill all %d buttons, got %d", deck.ButtonCount, len(manifest))
	}
	vp := manifest["1_1"].ViewParam[0]
	if vp.Text != "Grüße" || files[vp.Icon] == nil {
		t.Fatalf("button 6 manifest %+v", vp)
	}
	if manifest["1_0"].ViewParam[0].Icon != "" {
		t.Fatal("empty button must not reference an icon")
	}
}

func TestPartialZipOnlyContainsGivenButtons(t *testing.T) {
	blob, err := BuildButtonsZip([]deck.Button{{Index: deck.InfoWindowIndex, TextStyle: deck.DefaultTextStyle()}}, false)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := readZip(t, blob)
	var manifest map[string]manifestEntry
	_ = json.Unmarshal(files["manifest.json"], &manifest)
	if len(manifest) != 1 || manifest["3_2"].ViewParam[0].Icon == "" {
		t.Fatalf("manifest %v", manifest)
	}
}

func TestRejectsOutOfRangeIndex(t *testing.T) {
	if _, err := BuildButtonsZip([]deck.Button{{Index: 14}}, false); err == nil {
		t.Fatal("expected error")
	}
}

type fakeTransport struct {
	mu      sync.Mutex
	written [][]byte
	reads   chan []byte
	closed  chan struct{}
	once    sync.Once
	failOn  int
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{reads: make(chan []byte, 16), closed: make(chan struct{})}
}

func (f *fakeTransport) ReadReport() ([]byte, error) {
	select {
	case r := <-f.reads:
		return r, nil
	case <-f.closed:
		return nil, errors.New("closed")
	}
}

func (f *fakeTransport) WriteReport(frame []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn > 0 && len(f.written)+1 == f.failOn {
		return errors.New("unplugged")
	}
	f.written = append(f.written, append([]byte(nil), frame...))
	return nil
}

func (f *fakeTransport) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeTransport) commands() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var cmds []uint16
	for _, w := range f.written {
		if w[0] == 0x7C && w[1] == 0x7C {
			cmds = append(cmds, uint16(w[2])<<8|uint16(w[3]))
		}
	}
	return cmds
}

func TestDeviceCachesStateAndRestoresOnReconnect(t *testing.T) {
	first, second := newFakeTransport(), newFakeTransport()
	transports := make(chan Transport, 2)
	transports <- first
	transports <- second
	dev := NewDevice(func() (Transport, error) {
		select {
		case tr := <-transports:
			return tr, nil
		default:
			return nil, ErrDeviceNotFound
		}
	}, slog.New(slog.DiscardHandler))

	// Set before connecting: must only be cached.
	if err := dev.SetBrightness(70, false); !errors.Is(err, deck.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
	_ = dev.SetButtons([]deck.Button{{Index: 0, Label: "A", TextStyle: deck.DefaultTextStyle()}}, false)

	done := make(chan struct{})
	ctx, cancel := contextWithCancel()
	go func() { dev.Run(ctx); close(done) }()
	waitFor(t, dev.Connected)

	if got := first.commands(); len(got) < 1 || got[0] != uint16(CmdSetBrightness) {
		t.Fatalf("restore did not send brightness first: %v", got)
	}

	first.reads <- Frame(0x0101, 4, []byte{1, 5, 1, 1})
	ev := <-dev.Events()
	if c, ok := ev.(deck.ConnectionEvent); ok && c.Connected {
		ev = <-dev.Events()
	}
	if b, ok := ev.(deck.ButtonEvent); !ok || b.Index != 5 || !b.Pressed {
		t.Fatalf("event %#v", ev)
	}

	first.Close()
	waitFor(t, func() bool {
		cmds := second.commands()
		return len(cmds) >= 3
	})
	cmds := second.commands()
	if cmds[0] != uint16(CmdSetBrightness) || cmds[1] != uint16(CmdSetLabelStyle) || cmds[2] != uint16(CmdSetButtons) {
		t.Fatalf("restore order %v", cmds)
	}
	cancel()
	<-done
}
