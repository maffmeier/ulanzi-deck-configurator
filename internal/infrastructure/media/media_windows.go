package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

// Windows publishes "now playing" through the System Media Transport
// Controls, a WinRT API. It is called here through raw COM vtables, so no
// cgo or WinRT binding library is needed. Interface IDs and vtable slots
// come from the Windows metadata (as generated in microsoft/windows-rs).

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	iidSessionManagerStatics = guid{0x2050c4ee, 0x11a0, 0x57de, [8]byte{0xae, 0xd7, 0xc9, 0x7c, 0x70, 0x33, 0x82, 0x45}}
	iidAsyncInfo             = guid{0x00000036, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIStream               = guid{0x0000000c, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// Vtable slots: IUnknown 0-2, IInspectable 3-5, interface methods from 6.
const (
	slotQueryInterface = 0
	slotRelease        = 2

	slotStaticsRequestAsync = 6 // ISessionManagerStatics.RequestAsync

	slotManagerGetCurrentSession = 6 // ISessionManager.GetCurrentSession

	slotSessionTryGetMediaProperties = 7 // ISession.TryGetMediaPropertiesAsync
	slotSessionGetPlaybackInfo       = 9 // ISession.GetPlaybackInfo

	slotPropsTitle     = 6  // IMediaProperties.Title
	slotPropsArtist    = 9  // IMediaProperties.Artist
	slotPropsThumbnail = 15 // IMediaProperties.Thumbnail

	slotPlaybackStatus = 7 // IPlaybackInfo.PlaybackStatus

	slotAsyncInfoStatus     = 7 // IAsyncInfo.Status
	slotAsyncOpGetResults   = 8 // IAsyncOperation<T>.GetResults
	slotStreamRefOpenRead   = 6 // IRandomAccessStreamReference.OpenReadAsync
	slotIStreamRead         = 3 // ISequentialStream.Read
	playbackStatusPlaying   = 4
	asyncStatusStarted      = 0
	asyncStatusCompleted    = 1
	roInitMultithreaded     = 1
	rpcEChangedMode         = 0x80010106
	asyncTimeout            = 3 * time.Second
	sessionManagerClassName = "Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager"
	thumbnailScheme         = "smtc-thumbnail"
	maxThumbnailBytes       = 8 << 20
)

var (
	combase                           = syscall.NewLazyDLL("combase.dll")
	procRoInitialize                  = combase.NewProc("RoInitialize")
	procRoGetActivationFactory        = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString           = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString           = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer     = combase.NewProc("WindowsGetStringRawBuffer")
	procCreateStreamOverRandomAccessS = syscall.NewLazyDLL("shcore.dll").NewProc("CreateStreamOverRandomAccessStream")
)

type hresultError uintptr

func (h hresultError) Error() string { return fmt.Sprintf("HRESULT 0x%08X", uint32(h)) }

func check(hr uintptr) error {
	if int32(hr) < 0 {
		return hresultError(hr)
	}
	return nil
}

// asPointer reinterprets a pointer returned by Windows. It points into
// memory owned by COM, never into the Go heap, so the GC rules for
// uintptr conversions don't apply.
func asPointer(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

// call invokes vtable slot of a COM object.
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

// getObject calls a getter that returns an interface pointer.
func getObject(obj uintptr, slot int) (uintptr, error) {
	var out uintptr
	if err := check(call(obj, slot, uintptr(unsafe.Pointer(&out)))); err != nil {
		return 0, err
	}
	return out, nil
}

func newHString(s string) (uintptr, error) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return 0, err
	}
	var h uintptr
	hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	return h, check(hr)
}

func hstringValue(h uintptr) string {
	if h == 0 {
		return ""
	}
	var length uint32
	ptr, _, _ := procWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&length)))
	if ptr == 0 || length == 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(asPointer(ptr)), length))
}

func getString(obj uintptr, slot int) (string, error) {
	var h uintptr
	if err := check(call(obj, slot, uintptr(unsafe.Pointer(&h)))); err != nil {
		return "", err
	}
	defer procWindowsDeleteString.Call(h)
	return hstringValue(h), nil
}

// await polls an IAsyncOperation until it completes and returns its result.
func await(op uintptr) (uintptr, error) {
	defer release(op)
	var info uintptr
	if err := check(call(op, slotQueryInterface, uintptr(unsafe.Pointer(&iidAsyncInfo)), uintptr(unsafe.Pointer(&info)))); err != nil {
		return 0, err
	}
	defer release(info)
	deadline := time.Now().Add(asyncTimeout)
	for {
		var status int32
		if err := check(call(info, slotAsyncInfoStatus, uintptr(unsafe.Pointer(&status)))); err != nil {
			return 0, err
		}
		if status == asyncStatusCompleted {
			break
		}
		if status != asyncStatusStarted {
			return 0, fmt.Errorf("async operation ended with status %d", status)
		}
		if time.Now().After(deadline) {
			return 0, errors.New("async operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return getObject(op, slotAsyncOpGetResults)
}

// smtc owns the COM state; every call runs on one locked OS thread because
// COM objects belong to the apartment of the thread that created them.
type smtc struct {
	requests chan chan result
	manager  uintptr
	lastKey  string
	lastArt  string
}

type result struct {
	info Info
	err  error
}

var (
	smtcOnce     sync.Once
	smtcInstance *smtc
	thumbMu      sync.Mutex
	thumbImage   image.Image
)

func current() (Info, error) {
	smtcOnce.Do(func() {
		smtcInstance = &smtc{requests: make(chan chan result)}
		go smtcInstance.loop()
	})
	reply := make(chan result, 1)
	select {
	case smtcInstance.requests <- reply:
	case <-time.After(5 * time.Second):
		return Info{}, errors.New("media worker busy")
	}
	select {
	case r := <-reply:
		return r.info, r.err
	case <-time.After(5 * time.Second):
		return Info{}, errors.New("media query timed out")
	}
}

func (s *smtc) loop() {
	runtime.LockOSThread()
	hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
	initErr := check(hr)
	if uint32(hr) == rpcEChangedMode {
		initErr = nil
	}
	for reply := range s.requests {
		if initErr != nil {
			reply <- result{err: fmt.Errorf("RoInitialize: %w", initErr)}
			continue
		}
		info, err := s.query()
		reply <- result{info: info, err: err}
	}
}

func (s *smtc) ensureManager() error {
	if s.manager != 0 {
		return nil
	}
	name, err := newHString(sessionManagerClassName)
	if err != nil {
		return err
	}
	defer procWindowsDeleteString.Call(name)
	var statics uintptr
	hr, _, _ := procRoGetActivationFactory.Call(name, uintptr(unsafe.Pointer(&iidSessionManagerStatics)), uintptr(unsafe.Pointer(&statics)))
	if err := check(hr); err != nil {
		return fmt.Errorf("RoGetActivationFactory: %w", err)
	}
	defer release(statics)
	op, err := getObject(statics, slotStaticsRequestAsync)
	if err != nil {
		return err
	}
	manager, err := await(op)
	if err != nil {
		return err
	}
	s.manager = manager
	return nil
}

func (s *smtc) query() (Info, error) {
	if err := s.ensureManager(); err != nil {
		return Info{}, err
	}
	session, err := getObject(s.manager, slotManagerGetCurrentSession)
	if err != nil || session == 0 {
		return Info{}, err
	}
	defer release(session)

	var info Info
	if playback, err := getObject(session, slotSessionGetPlaybackInfo); err == nil && playback != 0 {
		var status int32
		if check(call(playback, slotPlaybackStatus, uintptr(unsafe.Pointer(&status)))) == nil {
			info.Playing = status == playbackStatusPlaying
		}
		release(playback)
	}

	op, err := getObject(session, slotSessionTryGetMediaProperties)
	if err != nil {
		return info, err
	}
	props, err := await(op)
	if err != nil || props == 0 {
		return info, err
	}
	defer release(props)
	info.Title, _ = getString(props, slotPropsTitle)
	info.Artist, _ = getString(props, slotPropsArtist)

	// The thumbnail is only read when the track changes.
	key := info.Title + "\x00" + info.Artist
	if key != s.lastKey {
		s.lastKey, s.lastArt = key, ""
		if img := readThumbnail(props); img != nil {
			thumbMu.Lock()
			thumbImage = img
			thumbMu.Unlock()
			// A distinct pseudo URL per track makes Cover() drop its cache.
			s.lastArt = thumbnailScheme + ":" + key
		}
	}
	info.ArtURL = s.lastArt
	return info, nil
}

func readThumbnail(props uintptr) image.Image {
	ref, err := getObject(props, slotPropsThumbnail)
	if err != nil || ref == 0 {
		return nil
	}
	defer release(ref)
	op, err := getObject(ref, slotStreamRefOpenRead)
	if err != nil {
		return nil
	}
	stream, err := await(op)
	if err != nil || stream == 0 {
		return nil
	}
	defer release(stream)

	var istream uintptr
	hr, _, _ := procCreateStreamOverRandomAccessS.Call(stream, uintptr(unsafe.Pointer(&iidIStream)), uintptr(unsafe.Pointer(&istream)))
	if check(hr) != nil || istream == 0 {
		return nil
	}
	defer release(istream)

	var data bytes.Buffer
	buf := make([]byte, 64<<10)
	for data.Len() < maxThumbnailBytes {
		var n uint32
		hr := call(istream, slotIStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
		if check(hr) != nil || n == 0 {
			break
		}
		data.Write(buf[:n])
	}
	img, err := render.DecodeImage(&data)
	if err != nil {
		return nil
	}
	return img
}

// platformCover resolves the pseudo URL current() hands out for covers.
func platformCover(raw string) (image.Image, bool) {
	if len(raw) <= len(thumbnailScheme)+1 || raw[:len(thumbnailScheme)+1] != thumbnailScheme+":" {
		return nil, false
	}
	thumbMu.Lock()
	defer thumbMu.Unlock()
	return thumbImage, true
}
