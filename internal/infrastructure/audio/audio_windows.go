package audio

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Core Audio through raw COM vtables, like the media package. Interface
// IDs and vtable slots come from mmdeviceapi.h, endpointvolume.h and
// audiopolicy.h; IPolicyConfig is the undocumented interface every audio
// switcher uses to change the default device.

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = guid{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}
	iidIAudioSessionManager = guid{0x77AA99A0, 0x1BD6, 0x484F, [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidIAudioSessionControl = guid{0xBFB7FF88, 0x7239, 0x4FC9, [8]byte{0x8F, 0xA2, 0x07, 0xC9, 0x50, 0xBE, 0x9C, 0x6D}}
	iidISimpleAudioVolume   = guid{0x87CE5498, 0x68D6, 0x44E5, [8]byte{0x92, 0x15, 0x6D, 0xA4, 0x7E, 0xF8, 0x83, 0xD8}}
	clsidPolicyConfigClient = guid{0x870AF99C, 0x171D, 0x4F9E, [8]byte{0xAF, 0x0D, 0xE6, 0x3D, 0xF4, 0x0C, 0x2B, 0xC9}}
	iidIPolicyConfig        = guid{0xF8679F50, 0x850A, 0x41CF, [8]byte{0x9C, 0x72, 0x43, 0x0F, 0x29, 0x02, 0x90, 0xC8}}
	pkeyDeviceFriendlyName  = propertyKey{guid{0xA45C254E, 0xDF1C, 0x4EFD, [8]byte{0x80, 0x20, 0x67, 0xD1, 0x46, 0xA8, 0x50, 0xE0}}, 14}
)

type propertyKey struct {
	fmtid guid
	pid   uint32
}

// propVariant is PROPVARIANT on 64-bit Windows (24 bytes).
type propVariant struct {
	vt       uint16
	_        [3]uint16
	val      uintptr
	_padding uintptr
}

const (
	slotQueryInterface = 0
	slotRelease        = 2

	slotEnumAudioEndpoints      = 3
	slotGetDefaultAudioEndpoint = 4
	slotDeviceActivate          = 3
	slotDeviceOpenPropertyStore = 4
	slotDeviceGetID             = 5
	slotCollectionGetCount      = 3
	slotCollectionItem          = 4
	slotPropertyStoreGetValue   = 5

	slotEndpointSetMute        = 14
	slotEndpointGetMute        = 15
	slotEndpointGetVolumeScale = 9
	slotEndpointStepInfo       = 16
	slotEndpointStepUp         = 17
	slotEndpointStepDown       = 18

	slotSessionManagerEnumerator = 5
	slotSessionEnumCount         = 3
	slotSessionEnumGet           = 4
	slotSessionControlProcessID  = 14

	slotSimpleSetVolume = 3
	slotSimpleGetVolume = 4
	slotSimpleSetMute   = 5
	slotSimpleGetMute   = 6

	slotPolicySetDefaultEndpoint = 13

	eRender, eCapture = 0, 1
	deviceStateActive = 1
	clsctxAll         = 0x17
	vtLPWSTR          = 31
	rpcEChangedMode   = 0x80010106
)

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procPropVariantClear = ole32.NewProc("PropVariantClear")
)

type hresult uintptr

func (h hresult) Error() string { return fmt.Sprintf("HRESULT 0x%08X", uint32(h)) }

func check(hr uintptr) error {
	if int32(hr) < 0 {
		return hresult(hr)
	}
	return nil
}

// asPointer reinterprets a pointer owned by COM, never by the Go heap.
func asPointer(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

func call(obj uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(asPointer(obj))
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return hr
}

func release(obj uintptr) {
	if obj != 0 {
		call(obj, slotRelease)
	}
}

func getObject(obj uintptr, slot int, args ...uintptr) (uintptr, error) {
	var out uintptr
	if err := check(call(obj, slot, append(args, uintptr(unsafe.Pointer(&out)))...)); err != nil {
		return 0, err
	}
	return out, nil
}

func queryInterface(obj uintptr, iid *guid) (uintptr, error) {
	return getObject(obj, slotQueryInterface, uintptr(unsafe.Pointer(iid)))
}

// withCOM runs fn on a locked thread with COM initialized.
func withCOM(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := procCoInitializeEx.Call(0, 0)
	if err := check(hr); err != nil && uint32(hr) != rpcEChangedMode {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	if int32(hr) >= 0 {
		defer procCoUninitialize.Call()
	}
	return fn()
}

func createInstance(clsid, iid *guid) (uintptr, error) {
	var out uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, clsctxAll, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, check(hr)
}

func withEnumerator(fn func(enum uintptr) error) error {
	return withCOM(func() error {
		enum, err := createInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
		if err != nil {
			return fmt.Errorf("MMDeviceEnumerator: %w", err)
		}
		defer release(enum)
		return fn(enum)
	})
}

func defaultDevice(enum uintptr, flow uintptr) (uintptr, error) {
	// eMultimedia: the device apps play to by default.
	return getObject(enum, slotGetDefaultAudioEndpoint, flow, 1)
}

func activate(device uintptr, iid *guid) (uintptr, error) {
	return getObject(device, slotDeviceActivate, uintptr(unsafe.Pointer(iid)), clsctxAll, 0)
}

func endpointVolume(enum uintptr, flow uintptr) (uintptr, error) {
	device, err := defaultDevice(enum, flow)
	if err != nil {
		return 0, err
	}
	defer release(device)
	return activate(device, &iidIAudioEndpointVolume)
}

func getBool(obj uintptr, slot int) (bool, error) {
	var v int32
	err := check(call(obj, slot, uintptr(unsafe.Pointer(&v))))
	return v != 0, err
}

func getFloat(obj uintptr, slot int) (float32, error) {
	var v float32
	err := check(call(obj, slot, uintptr(unsafe.Pointer(&v))))
	return v, err
}

func toggleEndpointMute(flow uintptr) error {
	return withEnumerator(func(enum uintptr) error {
		vol, err := endpointVolume(enum, flow)
		if err != nil {
			return err
		}
		defer release(vol)
		muted, err := getBool(vol, slotEndpointGetMute)
		if err != nil {
			return err
		}
		return check(call(vol, slotEndpointSetMute, boolArg(!muted), 0))
	})
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// session is one application's audio session on the default output.
type session struct {
	volume  uintptr
	process string
}

// sessions calls fn for every session whose process matches target.
func sessions(enum uintptr, target string, fn func(session) error) error {
	device, err := defaultDevice(enum, eRender)
	if err != nil {
		return err
	}
	defer release(device)
	manager, err := activate(device, &iidIAudioSessionManager)
	if err != nil {
		return err
	}
	defer release(manager)
	list, err := getObject(manager, slotSessionManagerEnumerator)
	if err != nil {
		return err
	}
	defer release(list)
	var count int32
	if err := check(call(list, slotSessionEnumCount, uintptr(unsafe.Pointer(&count)))); err != nil {
		return err
	}
	found := false
	for i := range count {
		control, err := getObject(list, slotSessionEnumGet, uintptr(i))
		if err != nil {
			continue
		}
		name := ""
		if control2, err := queryInterface(control, &iidIAudioSessionControl); err == nil {
			var pid uint32
			if check(call(control2, slotSessionControlProcessID, uintptr(unsafe.Pointer(&pid)))) == nil {
				name = processName(pid)
			}
			release(control2)
		}
		if name != "" && matchesApp(target, name) {
			if simple, err := queryInterface(control, &iidISimpleAudioVolume); err == nil {
				found = true
				err = fn(session{volume: simple, process: name})
				release(simple)
				if err != nil {
					release(control)
					return err
				}
			}
		}
		release(control)
	}
	if !found {
		return ErrAppNotFound
	}
	return nil
}

func processName(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(windows.UTF16ToString(buf[:size])), ".exe")
}

func changeVolume(target string, delta int) error {
	return withEnumerator(func(enum uintptr) error {
		if target != "" {
			return sessions(enum, target, func(s session) error {
				v, err := getFloat(s.volume, slotSimpleGetVolume)
				if err != nil {
					return err
				}
				next := float32(clampPercent(int(math.Round(float64(v)*100))+delta)) / 100
				return setSessionVolume(s.volume, next)
			})
		}
		vol, err := endpointVolume(enum, eRender)
		if err != nil {
			return err
		}
		defer release(vol)
		// Volume steps avoid passing a float through the syscall.
		var step, steps uint32
		if err := check(call(vol, slotEndpointStepInfo, uintptr(unsafe.Pointer(&step)), uintptr(unsafe.Pointer(&steps)))); err != nil || steps < 2 {
			steps = 51
		}
		n := max(1, int(math.Round(float64(abs(delta))*float64(steps-1)/100)))
		slot := slotEndpointStepUp
		if delta < 0 {
			slot = slotEndpointStepDown
		}
		for range n {
			if err := check(call(vol, slot, 0)); err != nil {
				return err
			}
		}
		return nil
	})
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func toggleMute(target string) error {
	if target == "" {
		return toggleEndpointMute(eRender)
	}
	return withEnumerator(func(enum uintptr) error {
		return sessions(enum, target, func(s session) error {
			muted, err := getBool(s.volume, slotSimpleGetMute)
			if err != nil {
				return err
			}
			return check(call(s.volume, slotSimpleSetMute, boolArg(!muted), 0))
		})
	})
}

func toggleMic() error {
	return toggleEndpointMute(eCapture)
}

type outputDevice struct {
	id, name string
}

func outputs(enum uintptr) ([]outputDevice, error) {
	coll, err := getObject(enum, slotEnumAudioEndpoints, eRender, deviceStateActive)
	if err != nil {
		return nil, err
	}
	defer release(coll)
	var count uint32
	if err := check(call(coll, slotCollectionGetCount, uintptr(unsafe.Pointer(&count)))); err != nil {
		return nil, err
	}
	var out []outputDevice
	for i := range count {
		device, err := getObject(coll, slotCollectionItem, uintptr(i))
		if err != nil {
			continue
		}
		if d, err := describe(device); err == nil {
			out = append(out, d)
		}
		release(device)
	}
	return out, nil
}

func describe(device uintptr) (outputDevice, error) {
	var idPtr uintptr
	if err := check(call(device, slotDeviceGetID, uintptr(unsafe.Pointer(&idPtr)))); err != nil {
		return outputDevice{}, err
	}
	id := windows.UTF16PtrToString((*uint16)(asPointer(idPtr)))
	procCoTaskMemFree.Call(idPtr)
	d := outputDevice{id: id, name: id}
	store, err := getObject(device, slotDeviceOpenPropertyStore, 0)
	if err != nil {
		return d, nil
	}
	defer release(store)
	var pv propVariant
	if check(call(store, slotPropertyStoreGetValue, uintptr(unsafe.Pointer(&pkeyDeviceFriendlyName)), uintptr(unsafe.Pointer(&pv)))) == nil {
		if pv.vt == vtLPWSTR && pv.val != 0 {
			d.name = windows.UTF16PtrToString((*uint16)(asPointer(pv.val)))
		}
		procPropVariantClear.Call(uintptr(unsafe.Pointer(&pv)))
	}
	return d, nil
}

func switchOutput(want string) error {
	return withEnumerator(func(enum uintptr) error {
		devices, err := outputs(enum)
		if err != nil {
			return err
		}
		names := make([]string, len(devices))
		for i, d := range devices {
			names[i] = d.name
		}
		currentName := ""
		if device, err := defaultDevice(enum, eRender); err == nil {
			if d, err := describe(device); err == nil {
				currentName = d.name
			}
			release(device)
		}
		i, err := nextDevice(names, currentName, want)
		if err != nil {
			return err
		}
		policy, err := createInstance(&clsidPolicyConfigClient, &iidIPolicyConfig)
		if err != nil {
			return fmt.Errorf("PolicyConfig: %w", err)
		}
		defer release(policy)
		id, err := windows.UTF16PtrFromString(devices[i].id)
		if err != nil {
			return err
		}
		for role := uintptr(0); role < 3; role++ {
			if err := check(call(policy, slotPolicySetDefaultEndpoint, uintptr(unsafe.Pointer(id)), role)); err != nil {
				return err
			}
		}
		return nil
	})
}

func current(target string) (State, error) {
	var st State
	err := withEnumerator(func(enum uintptr) error {
		vol, err := endpointVolume(enum, eRender)
		if err != nil {
			return err
		}
		defer release(vol)
		level, _ := getFloat(vol, slotEndpointGetVolumeScale)
		st.Volume = int(math.Round(float64(level) * 100))
		st.Muted, _ = getBool(vol, slotEndpointGetMute)
		st.AppFound = true
		if mic, err := endpointVolume(enum, eCapture); err == nil {
			st.MicMuted, _ = getBool(mic, slotEndpointGetMute)
			release(mic)
		}
		if device, err := defaultDevice(enum, eRender); err == nil {
			if d, err := describe(device); err == nil {
				st.Output = d.name
			}
			release(device)
		}
		if target == "" {
			return nil
		}
		first := true
		err = sessions(enum, target, func(s session) error {
			if first {
				v, _ := getFloat(s.volume, slotSimpleGetVolume)
				st.Volume = int(math.Round(float64(v) * 100))
				st.Muted, _ = getBool(s.volume, slotSimpleGetMute)
				first = false
			}
			return nil
		})
		if errors.Is(err, ErrAppNotFound) {
			st.AppFound, st.Volume, st.Muted = false, 0, false
			return nil
		}
		return err
	})
	return st, err
}
