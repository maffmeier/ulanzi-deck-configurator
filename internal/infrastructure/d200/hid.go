package d200

import (
	"errors"
	"fmt"

	"rafaelmartins.com/p/usbhid"

	"github.com/maffmeier/ulanzi-deck/internal/domain/deck"
)

var ErrDeviceNotFound = errors.New("no Ulanzi D200 found")

type DeviceInfo struct {
	Path         string
	Manufacturer string
	Product      string
	Serial       string
}

// The D200 exposes a vendor interface with 1024-byte reports and a boot
// keyboard interface; only the former speaks the deck protocol.
func isDeckInterface(d *usbhid.Device) bool {
	return d.VendorId() == deck.VendorID &&
		d.ProductId() == deck.ProductID &&
		d.GetOutputReportLength() >= PacketSize
}

func Enumerate() ([]DeviceInfo, error) {
	devices, err := usbhid.Enumerate(isDeckInterface)
	if err != nil {
		return nil, err
	}
	infos := make([]DeviceInfo, 0, len(devices))
	for _, d := range devices {
		infos = append(infos, DeviceInfo{
			Path:         d.Path(),
			Manufacturer: d.Manufacturer(),
			Product:      d.Product(),
			Serial:       d.SerialNumber(),
		})
	}
	return infos, nil
}

type hidTransport struct {
	dev *usbhid.Device
}

// OpenHID opens the first D200 that is attached.
func OpenHID() (Transport, error) {
	devices, err := usbhid.Enumerate(isDeckInterface)
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, ErrDeviceNotFound
	}
	var errs []error
	for _, dev := range devices {
		if err := dev.Open(false); err != nil {
			errs = append(errs, err)
			continue
		}
		return &hidTransport{dev: dev}, nil
	}
	return nil, fmt.Errorf("cannot open deck (permissions?): %w", errors.Join(errs...))
}

func (h *hidTransport) ReadReport() ([]byte, error) {
	_, data, err := h.dev.GetInputReport()
	return data, err
}

func (h *hidTransport) WriteReport(frame []byte) error {
	return h.dev.SetOutputReport(0, frame)
}

func (h *hidTransport) Close() error {
	if !h.dev.IsOpen() {
		return nil
	}
	return h.dev.Close()
}
