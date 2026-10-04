// Package d200 implements the reverse-engineered HID protocol of the Ulanzi
// Stream Controller D200 (see docs/protocol.md of ulanzi-linux).
package d200

import (
	"bytes"
	"encoding/binary"
)

const (
	PacketSize     = 1024
	HeaderSize     = 8
	MaxPayloadSize = PacketSize - HeaderSize
)

type OutCommand uint16

const (
	CmdSetButtons         OutCommand = 0x0001
	CmdSetSmallWindowData OutCommand = 0x0006
	CmdSetBrightness      OutCommand = 0x000A
	CmdSetLabelStyle      OutCommand = 0x000B
	CmdPartialButtons     OutCommand = 0x000D
)

func (c OutCommand) String() string {
	switch c {
	case CmdSetButtons:
		return "SET_BUTTONS"
	case CmdSetSmallWindowData:
		return "SET_SMALL_WINDOW_DATA"
	case CmdSetBrightness:
		return "SET_BRIGHTNESS"
	case CmdSetLabelStyle:
		return "SET_LABEL_STYLE"
	case CmdPartialButtons:
		return "PARTIALLY_UPDATE_BUTTONS"
	}
	return "UNKNOWN"
}

const (
	inButton     = 0x0101
	inDeviceInfo = 0x0303
)

// Frame builds a 1024-byte packet: magic 0x7C7C, big-endian command, and a
// length field the firmware reads byte-swapped (i.e. little-endian).
func Frame(cmd OutCommand, length int, data []byte) []byte {
	frame := make([]byte, PacketSize)
	frame[0], frame[1] = 0x7C, 0x7C
	binary.BigEndian.PutUint16(frame[2:4], uint16(cmd))
	binary.LittleEndian.PutUint32(frame[4:8], uint32(length))
	copy(frame[HeaderSize:], data)
	return frame
}

// Chunks splits a large payload: the first frame carries the header with
// the TOTAL length, continuation frames are raw 1024-byte slices without
// any header. Wrapping continuation frames wedges the firmware.
func Chunks(cmd OutCommand, blob []byte) [][]byte {
	first := blob[:min(len(blob), MaxPayloadSize)]
	frames := [][]byte{Frame(cmd, len(blob), first)}
	for offset := MaxPayloadSize; offset < len(blob); offset += PacketSize {
		chunk := make([]byte, PacketSize)
		copy(chunk, blob[offset:min(len(blob), offset+PacketSize)])
		frames = append(frames, chunk)
	}
	return frames
}

type incoming struct {
	command uint16
	data    []byte
}

func parseIncoming(raw []byte) (incoming, bool) {
	if len(raw) < HeaderSize {
		return incoming{}, false
	}
	return incoming{
		command: binary.BigEndian.Uint16(raw[2:4]),
		data:    raw[HeaderSize:],
	}, true
}

func cString(data []byte) string {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		data = data[:i]
	}
	return string(data)
}
