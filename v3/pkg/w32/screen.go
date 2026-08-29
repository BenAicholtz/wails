//go:build windows

package w32

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

type Screen struct {
	MONITORINFOEX
	HMonitor    uintptr
	Name        string
	IsPrimary   bool
	IsCurrent   bool
	ScaleFactor float32
	Rotation    float32
}

type DISPLAY_DEVICE struct {
	cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

func getMonitorName(deviceName string) (string, error) {
	var device DISPLAY_DEVICE
	device.cb = uint32(unsafe.Sizeof(device))
	i := uint32(0)
	for {
		res, _, _ := procEnumDisplayDevices.Call(uintptr(unsafe.Pointer(MustStringToUTF16Ptr(deviceName))), uintptr(i), uintptr(unsafe.Pointer(&device)), 0)
		if res == 0 {
			break
		}
		if device.StateFlags&0x1 != 0 {
			return syscall.UTF16ToString(device.DeviceString[:]), nil
		}
		i++
	}

	return "", fmt.Errorf("monitor name not found for device: %s", deviceName)
}

// I'm not convinced this works properly
func GetRotationForMonitor(displayName [32]uint16) (float32, error) {
	var devMode DEVMODE
	devMode.DmSize = uint16(unsafe.Sizeof(devMode))
	resp, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&displayName[0])), ENUM_CURRENT_SETTINGS, uintptr(unsafe.Pointer(&devMode)))
	if resp == 0 {
		return 0, fmt.Errorf("EnumDisplaySettings failed")
	}

	if (devMode.DmFields & DM_DISPLAYORIENTATION) == 0 {
		return 0, fmt.Errorf("DM_DISPLAYORIENTATION not set")
	}

	switch devMode.DmOrientation {
	case DMDO_DEFAULT:
		return 0, nil
	case DMDO_90:
		return 90, nil
	case DMDO_180:
		return 180, nil
	case DMDO_270:
		return 270, nil
	}

	return -1, nil
}

// cursorPosForScreens reports the current cursor position and whether the
// query succeeded. GetCursorPos returns FALSE (ERROR_ACCESS_DENIED) when the
// calling process is not attached to the interactive input desktop - e.g. the
// workstation is locked or on the secure/UAC desktop, or mid session switch
// (logon, RDP, fast-user-switch). Exposed as a package variable so tests can
// simulate that failure. Defaults to the real GetCursorPos syscall.
var cursorPosForScreens = func() (cursor POINT, ok bool) {
	ret, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
	return cursor, ret != 0
}

// enumScreens holds the per-enumeration state and the singleton callback used
// by GetAllScreens.
//
// syscall.NewCallback registers a callback in a fixed-size, process-lifetime
// table that Go never frees, and the process is capped at ~2000 callbacks
// total. Creating a fresh callback on every GetAllScreens call leaks one slot
// per call, so a long-running app that re-enumerates monitors on every
// WM_DISPLAYCHANGE / WM_SETTINGCHANGE eventually exhausts the table and dies
// with "fatal error: too many callback functions".
//
// The callback is created exactly once. EnumDisplayMonitors runs the callback
// synchronously on the calling thread before returning, so a single mutex is
// enough to make the shared state safe across concurrent callers.
var enumScreens struct {
	mu       sync.Mutex
	once     sync.Once
	callback uintptr
	result   []*Screen
	errMsg   string
	cursor   POINT
	cursorOK bool
}

// enumScreensProc is the MONITORENUMPROC body. It must only be invoked while
// enumScreens.mu is held (i.e. from within GetAllScreens).
func enumScreensProc(hMonitor uintptr, hdc uintptr, lprcMonitor *RECT, lParam uintptr) uintptr {
	monitor := MONITORINFOEX{
		MONITORINFO: MONITORINFO{
			CbSize: uint32(unsafe.Sizeof(MONITORINFOEX{})),
		},
		SzDevice: [32]uint16{},
	}
	ret, _, _ := procGetMonitorInfo.Call(hMonitor, uintptr(unsafe.Pointer(&monitor)))
	if ret == 0 {
		enumScreens.errMsg = "GetMonitorInfo failed"
		return 0 // Stop enumeration
	}

	screen := &Screen{
		MONITORINFOEX: monitor,
		HMonitor:      hMonitor,
		IsPrimary:     monitor.DwFlags == MONITORINFOF_PRIMARY,
		IsCurrent:     enumScreens.cursorOK && rectContainsPoint(monitor.RcMonitor, enumScreens.cursor),
	}

	// Get monitor name
	name, err := getMonitorName(syscall.UTF16ToString(monitor.SzDevice[:]))
	if err == nil {
		screen.Name = name
	}

	// Get DPI for monitor
	var dpiX, dpiY uint
	ret = GetDPIForMonitor(hMonitor, MDT_EFFECTIVE_DPI, &dpiX, &dpiY)
	if ret != S_OK {
		enumScreens.errMsg = "GetDpiForMonitor failed"
		return 0 // Stop enumeration
	}
	// Convert to scale factor
	screen.ScaleFactor = float32(dpiX) / 96.0

	// Get rotation of monitor
	rot, err := GetRotationForMonitor(monitor.SzDevice)
	if err == nil {
		screen.Rotation = rot
	}

	enumScreens.result = append(enumScreens.result, screen)
	return 1 // Continue enumeration
}

func GetAllScreens() ([]*Screen, error) {
	enumScreens.once.Do(func() {
		enumScreens.callback = syscall.NewCallback(enumScreensProc)
	})

	enumScreens.mu.Lock()
	defer func() {
		// Don't pin the collected screens (or a stale cursor) for the
		// lifetime of the process.
		enumScreens.result = nil
		enumScreens.errMsg = ""
		enumScreens.mu.Unlock()
	}()

	enumScreens.result = nil
	enumScreens.errMsg = ""

	// The cursor position is used only to set the cosmetic IsCurrent flag
	// (which monitor the pointer is on). A failed query must NOT abort
	// enumeration: processAndCacheScreens() treats any error here as fatal at
	// startup (os.Exit), so a locked/secure desktop at launch would crash the
	// app before it ever shows. Degrade gracefully - no screen is marked
	// current, and the flag self-corrects on the next display-change re-cache.
	enumScreens.cursor, enumScreens.cursorOK = cursorPosForScreens()

	ret, _, _ := procEnumDisplayMonitors.Call(0, 0, enumScreens.callback, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors failed: %s", enumScreens.errMsg)
	}

	return enumScreens.result, nil
}

func rectContainsPoint(r RECT, p POINT) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}
