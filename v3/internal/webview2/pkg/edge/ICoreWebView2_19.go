//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL mirrors the WebView2 SDK enum.
type COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL uint32

const (
	COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL = 0
	COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_LOW    COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL = 1
)

// iCoreWebView2_19Vtbl is the full ICoreWebView2_19 vtable: everything up to
// ICoreWebView2_3 (73 slots, declared in iCoreWebView2_3Vtbl), then the 46
// methods added by ICoreWebView2_4 through _18 (4+2+1+1+7+9+2+3+3+1+3+4+3+1+2),
// which nothing here calls, then the two _19 methods at slots 119 and 120.
//
// The generated pkg/webview2.ICoreWebView2_19Vtbl embeds only IUnknown and
// would call the wrong slots; do not copy it.
type iCoreWebView2_19Vtbl struct {
	iCoreWebView2_3Vtbl
	_                         [46]ComProc
	GetMemoryUsageTargetLevel ComProc
	PutMemoryUsageTargetLevel ComProc
}

// Compile-time layout check: fails to build if the getter is not slot 119.
var _ = [1]struct{}{}[unsafe.Offsetof(iCoreWebView2_19Vtbl{}.GetMemoryUsageTargetLevel)/unsafe.Sizeof(ComProc(0))-119]

type ICoreWebView2_19 struct {
	vtbl *iCoreWebView2_19Vtbl
}

func (i *ICoreWebView2_19) Release() uint32 {
	ret, _, _ := i.vtbl.Release.Call(uintptr(unsafe.Pointer(i)))
	return uint32(ret)
}

func (i *ICoreWebView2_19) PutMemoryUsageTargetLevel(level COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL) error {
	hr, _, _ := i.vtbl.PutMemoryUsageTargetLevel.Call(uintptr(unsafe.Pointer(i)), uintptr(level))
	if windows.Handle(hr) != windows.S_OK {
		return windows.Errno(hr)
	}
	return nil
}

func (i *ICoreWebView2_19) GetMemoryUsageTargetLevel() (COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL, error) {
	var level COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL
	hr, _, _ := i.vtbl.GetMemoryUsageTargetLevel.Call(uintptr(unsafe.Pointer(i)), uintptr(unsafe.Pointer(&level)))
	if windows.Handle(hr) != windows.S_OK {
		return 0, windows.Errno(hr)
	}
	return level, nil
}

// GetICoreWebView2_19 returns nil when the installed runtime predates the
// interface (WebView2 runtime 114 and older). The caller must Release it.
func (i *ICoreWebView2) GetICoreWebView2_19() *ICoreWebView2_19 {
	var result *ICoreWebView2_19
	iid := NewGUID("{6921F954-79B0-437F-A997-C85811897C68}")
	_, _, _ = i.vtbl.QueryInterface.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&result)))
	return result
}

// SetMemoryUsageTargetLevel lowers or restores the WebView's memory target.
// LOW lets WebView2 trim caches and release what it can while the app is in
// the background; script keeps running. Must be called on the UI thread.
func (e *Chromium) SetMemoryUsageTargetLevel(level COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL) error {
	if e == nil || e.webview == nil {
		return UnsupportedCapabilityError
	}
	wv19 := e.webview.GetICoreWebView2_19()
	if wv19 == nil {
		return UnsupportedCapabilityError
	}
	defer wv19.Release()
	return wv19.PutMemoryUsageTargetLevel(level)
}
