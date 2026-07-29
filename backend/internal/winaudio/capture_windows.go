//go:build windows

// Package winaudio captures the audio rendered by one Windows process tree.
//
// Windows 10 build 20348 introduced process-loopback activation for WASAPI.
// Unlike endpoint loopback, this virtual device follows a target process and
// all of its descendants across physical output devices. That is exactly the
// Chromium model: the browser process owns renderer and audio-service child
// processes, while unrelated host audio must not reach a Surf client.
package winaudio

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	virtualProcessLoopback = `VAD\Process_Loopback`

	audioClientActivationTypeProcessLoopback = 1
	processLoopbackIncludeTargetProcessTree  = 0
	propVariantTypeBlob                      = 65

	audioClientShareModeShared              = 0
	audioClientStreamFlagsLoopback          = 0x00020000
	audioClientStreamFlagsEventCallback     = 0x00040000
	audioClientStreamFlagsSrcDefaultQuality = 0x08000000
	audioClientStreamFlagsAutoConvertPCM    = 0x80000000
	audioClientBufferFlagsSilent            = 0x00000002

	waveFormatPCM = 1
	sampleRate    = 16000
	channels      = 1
	bitsPerSample = 16

	sOK          = 0
	eNoInterface = 0x80004002
)

var (
	mmdevapi                        = windows.NewLazySystemDLL("mmdevapi.dll")
	procActivateAudioInterfaceAsync = mmdevapi.NewProc("ActivateAudioInterfaceAsync")

	iidIUnknown = windows.GUID{
		Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000,
		Data4: [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidIAgileObject = windows.GUID{
		Data1: 0x94ea2b94, Data2: 0xe9cc, Data3: 0x49e0,
		Data4: [8]byte{0xc0, 0xff, 0xee, 0x64, 0xca, 0x8f, 0x5b, 0x90},
	}
	iidActivationCompletionHandler = windows.GUID{
		Data1: 0x41d949ab, Data2: 0x9862, Data3: 0x444a,
		Data4: [8]byte{0x80, 0xf6, 0xc2, 0x61, 0x33, 0x4d, 0xa5, 0xeb},
	}
	iidAudioClient = windows.GUID{
		Data1: 0x1cb9ad4c, Data2: 0xdbfa, Data3: 0x4c32,
		Data4: [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2},
	}
	iidAudioCaptureClient = windows.GUID{
		Data1: 0xc8adbd64, Data2: 0xe71e, Data3: 0x48a0,
		Data4: [8]byte{0xa4, 0xde, 0x18, 0x5c, 0x39, 0x5c, 0xd3, 0x17},
	}
)

type processLoopbackParams struct {
	TargetProcessID uint32
	Mode            uint32
}

type audioClientActivationParams struct {
	ActivationType uint32
	Process        processLoopbackParams
}

type blob struct {
	Size uint32
	_    uint32 // align Data like PROPVARIANT's BLOB on windows/amd64
	Data *byte
}

type propVariant struct {
	Type      uint16
	Reserved1 uint16
	Reserved2 uint16
	Reserved3 uint16
	Blob      blob
}

type waveFormat struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	ExtraSize      uint16
}

type comObject struct {
	vtable *[15]uintptr
}

type activationResult struct {
	audioClient *comObject
	err         error
}

type completionHandler struct {
	vtable *completionHandlerVTable
	refs   atomic.Uint32
	result chan activationResult
}

type completionHandlerVTable struct {
	queryInterface    uintptr
	addRef            uintptr
	release           uintptr
	activateCompleted uintptr
}

var completionVTable = completionHandlerVTable{
	queryInterface:    windows.NewCallback(completionQueryInterface),
	addRef:            windows.NewCallback(completionAddRef),
	release:           windows.NewCallback(completionRelease),
	activateCompleted: windows.NewCallback(completionActivateCompleted),
}

func completionQueryInterface(this *completionHandler, requestedIID *windows.GUID, object *unsafe.Pointer) uintptr {
	if object == nil || requestedIID == nil {
		return eNoInterface
	}
	requested := *requestedIID
	if requested != iidIUnknown && requested != iidActivationCompletionHandler && requested != iidIAgileObject {
		*object = nil
		return eNoInterface
	}
	*object = unsafe.Pointer(this)
	completionAddRef(this)
	return sOK
}

func completionAddRef(this *completionHandler) uintptr {
	return uintptr(this.refs.Add(1))
}

func completionRelease(this *completionHandler) uintptr {
	for {
		current := this.refs.Load()
		if current == 0 {
			return 0
		}
		if this.refs.CompareAndSwap(current, current-1) {
			return uintptr(current - 1)
		}
	}
}

func completionActivateCompleted(this *completionHandler, operation *comObject) uintptr {
	var activationHR int32
	var audioClient *comObject
	callHR, _ := callCOM(operation, 3,
		uintptr(unsafe.Pointer(&activationHR)),
		uintptr(unsafe.Pointer(&audioClient)),
	)
	var err error
	if failed(callHR) {
		err = hresultError("IActivateAudioInterfaceAsyncOperation.GetActivateResult", callHR)
	} else if failed(uintptr(uint32(activationHR))) {
		err = hresultError("process-loopback activation", uintptr(uint32(activationHR)))
	} else if audioClient == nil {
		err = fmt.Errorf("process-loopback activation returned no IAudioClient")
	}
	select {
	case this.result <- activationResult{audioClient: audioClient, err: err}:
	default:
		if audioClient != nil {
			releaseCOM(audioClient)
		}
	}
	return sOK
}

// Capture is a live signed-16-bit, 16 kHz mono PCM stream.
type Capture struct {
	reader *io.PipeReader
	stop   windows.Handle
	done   chan struct{}
	once   sync.Once
}

// OpenProcessLoopback starts capturing audio rendered by processID and every
// descendant process. Capture is independent of the selected output device.
func OpenProcessLoopback(processID uint32) (*Capture, error) {
	if processID == 0 {
		return nil, fmt.Errorf("process-loopback capture requires a browser process ID")
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("create audio stop event: %w", err)
	}
	reader, writer := io.Pipe()
	capture := &Capture{reader: reader, stop: stop, done: make(chan struct{})}
	ready := make(chan error, 1)
	go capture.run(processID, writer, ready)
	if err := <-ready; err != nil {
		_ = reader.Close()
		_ = windows.CloseHandle(stop)
		return nil, err
	}
	return capture, nil
}

func (c *Capture) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *Capture) Close() error {
	var err error
	c.once.Do(func() {
		if setErr := windows.SetEvent(c.stop); setErr != nil {
			err = setErr
		}
		_ = c.reader.Close()
		<-c.done
		_ = windows.CloseHandle(c.stop)
	})
	return err
}

func (c *Capture) run(processID uint32, writer *io.PipeWriter, ready chan<- error) {
	defer close(c.done)
	defer writer.Close()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		ready <- fmt.Errorf("initialize COM for process-loopback capture: %w", err)
		return
	}
	defer windows.CoUninitialize()

	audioClient, err := activate(processID)
	if err != nil {
		ready <- err
		return
	}
	defer releaseCOM(audioClient)

	sampleReady, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		ready <- fmt.Errorf("create audio sample event: %w", err)
		return
	}
	defer windows.CloseHandle(sampleReady)

	format := waveFormat{
		FormatTag: waveFormatPCM, Channels: channels,
		SamplesPerSec: sampleRate, BitsPerSample: bitsPerSample,
		BlockAlign: channels * bitsPerSample / 8,
	}
	format.AvgBytesPerSec = format.SamplesPerSec * uint32(format.BlockAlign)
	flags := uint32(audioClientStreamFlagsLoopback |
		audioClientStreamFlagsEventCallback |
		audioClientStreamFlagsSrcDefaultQuality |
		audioClientStreamFlagsAutoConvertPCM)
	if hr, _ := callCOM(audioClient, 3,
		audioClientShareModeShared,
		uintptr(flags),
		0,
		0,
		uintptr(unsafe.Pointer(&format)),
		0,
	); failed(hr) {
		ready <- hresultError("IAudioClient.Initialize", hr)
		return
	}
	if hr, _ := callCOM(audioClient, 13, uintptr(sampleReady)); failed(hr) {
		ready <- hresultError("IAudioClient.SetEventHandle", hr)
		return
	}
	var captureClient *comObject
	if hr, _ := callCOM(audioClient, 14,
		uintptr(unsafe.Pointer(&iidAudioCaptureClient)),
		uintptr(unsafe.Pointer(&captureClient)),
	); failed(hr) {
		ready <- hresultError("IAudioClient.GetService(IAudioCaptureClient)", hr)
		return
	}
	if captureClient == nil {
		ready <- fmt.Errorf("IAudioClient.GetService returned no IAudioCaptureClient")
		return
	}
	defer releaseCOM(captureClient)

	if hr, _ := callCOM(audioClient, 10); failed(hr) {
		ready <- hresultError("IAudioClient.Start", hr)
		return
	}
	defer callCOM(audioClient, 11)
	ready <- nil

	handles := []windows.Handle{c.stop, sampleReady}
	for {
		wait, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
		if err != nil {
			_ = writer.CloseWithError(fmt.Errorf("wait for process-loopback audio: %w", err))
			return
		}
		switch wait {
		case windows.WAIT_OBJECT_0:
			return
		case windows.WAIT_OBJECT_0 + 1:
			if err := drain(captureClient, writer, format.BlockAlign); err != nil {
				_ = writer.CloseWithError(err)
				return
			}
		default:
			_ = writer.CloseWithError(fmt.Errorf("unexpected process-loopback wait result %#x", wait))
			return
		}
	}
}

func activate(processID uint32) (*comObject, error) {
	params := audioClientActivationParams{
		ActivationType: audioClientActivationTypeProcessLoopback,
		Process: processLoopbackParams{
			TargetProcessID: processID,
			Mode:            processLoopbackIncludeTargetProcessTree,
		},
	}
	variant := propVariant{
		Type: propVariantTypeBlob,
		Blob: blob{
			Size: uint32(unsafe.Sizeof(params)),
			Data: (*byte)(unsafe.Pointer(&params)),
		},
	}
	handler := &completionHandler{
		vtable: &completionVTable,
		result: make(chan activationResult, 1),
	}
	handler.refs.Store(1)
	device, err := windows.UTF16PtrFromString(virtualProcessLoopback)
	if err != nil {
		return nil, err
	}
	var operation *comObject
	hr, _, _ := procActivateAudioInterfaceAsync.Call(
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(&iidAudioClient)),
		uintptr(unsafe.Pointer(&variant)),
		uintptr(unsafe.Pointer(handler)),
		uintptr(unsafe.Pointer(&operation)),
	)
	if failed(hr) {
		return nil, hresultError("ActivateAudioInterfaceAsync", hr)
	}
	result := <-handler.result
	if operation != nil {
		releaseCOM(operation)
	}
	runtime.KeepAlive(handler)
	runtime.KeepAlive(params)
	runtime.KeepAlive(variant)
	return result.audioClient, result.err
}

func drain(captureClient *comObject, writer *io.PipeWriter, blockAlign uint16) error {
	for {
		var frames uint32
		if hr, _ := callCOM(captureClient, 5, uintptr(unsafe.Pointer(&frames))); failed(hr) {
			return hresultError("IAudioCaptureClient.GetNextPacketSize", hr)
		}
		if frames == 0 {
			return nil
		}
		var data *byte
		var flags uint32
		if hr, _ := callCOM(captureClient, 3,
			uintptr(unsafe.Pointer(&data)),
			uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)),
			0,
			0,
		); failed(hr) {
			return hresultError("IAudioCaptureClient.GetBuffer", hr)
		}
		size := int(frames) * int(blockAlign)
		packet := make([]byte, size)
		if flags&audioClientBufferFlagsSilent == 0 && size != 0 {
			if data == nil {
				_, _ = callCOM(captureClient, 4, uintptr(frames))
				return fmt.Errorf("IAudioCaptureClient.GetBuffer returned nil data for %d frames", frames)
			}
			copy(packet, unsafe.Slice(data, size))
		}
		if hr, _ := callCOM(captureClient, 4, uintptr(frames)); failed(hr) {
			return hresultError("IAudioCaptureClient.ReleaseBuffer", hr)
		}
		if _, err := writer.Write(packet); err != nil {
			return err
		}
	}
}

func callCOM(object *comObject, method int, args ...uintptr) (uintptr, error) {
	if object == nil {
		return eNoInterface, fmt.Errorf("nil COM object")
	}
	if object.vtable == nil {
		return eNoInterface, fmt.Errorf("nil COM vtable")
	}
	if method < 0 || method >= len(object.vtable) {
		return eNoInterface, fmt.Errorf("COM method index %d is out of range", method)
	}
	fn := object.vtable[method]
	callArgs := make([]uintptr, 1, len(args)+1)
	callArgs[0] = uintptr(unsafe.Pointer(object))
	callArgs = append(callArgs, args...)
	result, _, callErr := syscall.SyscallN(fn, callArgs...)
	return result, callErr
}

func releaseCOM(object *comObject) {
	if object != nil {
		_, _ = callCOM(object, 2)
	}
}

func failed(hr uintptr) bool {
	return int32(uint32(hr)) < 0
}

func hresultError(operation string, hr uintptr) error {
	return fmt.Errorf("%s failed: HRESULT 0x%08x", operation, uint32(hr))
}
