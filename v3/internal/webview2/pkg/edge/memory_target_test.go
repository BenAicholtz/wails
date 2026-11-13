//go:build windows

package edge

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/internal/webview2/internal/w32"
	"golang.org/x/sys/windows"
)

// TestMemoryUsageTargetLevel round-trips MemoryUsageTargetLevel on a real
// WebView2. The ICoreWebView2_19 vtable is hand-built (46 padding slots after
// _3); a wrong slot would call some other method and fail or crash here.
// Same window/apartment setup as TestCookieManager.
func TestMemoryUsageTargetLevel(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		t.Fatalf("Failed to initialize COM: %v", err)
	}
	defer windows.CoUninitialize()

	var hinstance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinstance)
	className, _ := windows.UTF16PtrFromString("WebView2MemoryTargetTest")
	wc := w32.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(w32.WndClassExW{})),
		HInstance:     hinstance,
		LpszClassName: className,
		LpfnWndProc:   windows.NewCallback(w32.DefWindowProc),
	}
	_, _, _ = w32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	windowName, _ := windows.UTF16PtrFromString("WebView2 Memory Target Test")
	hwnd, _, _ := w32.User32CreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0xCF0000, // WS_OVERLAPPEDWINDOW
		uintptr(w32.CW_USEDEFAULT),
		uintptr(w32.CW_USEDEFAULT),
		640,
		480,
		0,
		0,
		uintptr(hinstance),
		0,
	)
	if hwnd == 0 {
		t.Fatal("Failed to create window")
	}
	defer w32.DestroyWindow(hwnd)

	chromium := NewChromium()
	require.True(t, chromium.Embed(uintptr(hwnd)), "WebView2 initialization should succeed")

	wv19 := chromium.webview.GetICoreWebView2_19()
	if wv19 == nil {
		t.Skip("installed WebView2 runtime predates ICoreWebView2_19")
	}
	defer wv19.Release()

	level, err := wv19.GetMemoryUsageTargetLevel()
	require.NoError(t, err)
	require.Equal(t, COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL, level, "a new WebView starts at NORMAL")

	require.NoError(t, chromium.SetMemoryUsageTargetLevel(COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_LOW))
	level, err = wv19.GetMemoryUsageTargetLevel()
	require.NoError(t, err)
	require.Equal(t, COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_LOW, level)

	require.NoError(t, chromium.SetMemoryUsageTargetLevel(COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL))
	level, err = wv19.GetMemoryUsageTargetLevel()
	require.NoError(t, err)
	require.Equal(t, COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL, level)
}
