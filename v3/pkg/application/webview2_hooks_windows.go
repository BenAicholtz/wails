//go:build windows

package application

import (
	"log"
	"time"

	"github.com/wailsapp/wails/v3/internal/webview2/pkg/edge"
)

// WebView2ProcessFailedKind identifies which WebView2 child process died. It
// mirrors COREWEBVIEW2_PROCESS_FAILED_KIND, re-exported here because the edge
// package is internal to Wails.
type WebView2ProcessFailedKind uint32

const (
	WebView2ProcessFailedBrowserProcessExited      WebView2ProcessFailedKind = 0
	WebView2ProcessFailedRenderProcessExited       WebView2ProcessFailedKind = 1
	WebView2ProcessFailedRenderProcessUnresponsive WebView2ProcessFailedKind = 2
	WebView2ProcessFailedFrameRenderProcessExited  WebView2ProcessFailedKind = 3
	WebView2ProcessFailedUtilityProcessExited      WebView2ProcessFailedKind = 4
	WebView2ProcessFailedSandboxHelperExited       WebView2ProcessFailedKind = 5
	WebView2ProcessFailedGPUProcessExited          WebView2ProcessFailedKind = 6
	WebView2ProcessFailedPPAPIPluginExited         WebView2ProcessFailedKind = 7
	WebView2ProcessFailedPPAPIBrokerExited         WebView2ProcessFailedKind = 8
	WebView2ProcessFailedUnknownProcessExited      WebView2ProcessFailedKind = 9
)

// SetWebView2ErrorExitOnReturn controls whether an unrecoverable WebView2 error
// terminates the process (os.Exit(1)) once Options.ErrorHandler returns.
// Defaults to true. Pass false when the application's error handler can absorb
// transient runtime failures — a GPU TDR, for instance, surfaces as a burst of
// 0x8007139F errors that WebView2 recovers from on its own — and wants to
// decide for itself when a failure is actually fatal.
func SetWebView2ErrorExitOnReturn(exit bool) {
	edge.SetErrorExitOnReturn(exit)
}

// SetWebView2OpenExternalURLHook installs a handler for WebView2 new-window
// requests: window.open, target="_blank", and links clicked inside cross-origin
// iframes. Returning true means the application has opened the URL itself (in
// the user's default browser, say) and WebView2 must not spawn its own bare
// popup window; returning false leaves WebView2's default behaviour in place.
//
// Only http, https and mailto URLs reach the hook, and programmatic popups
// (window.open with no user gesture) are dropped before it is consulted.
//
// Pass nil to restore stock behaviour.
func SetWebView2OpenExternalURLHook(hook func(url string) bool) {
	edge.OpenExternalURLHook = hook
}

// SetWebView2ProcessFailedRecoveryHook installs a handler that runs before
// Wails' own WebView2 process-failure handling. Returning true marks the
// failure as handled, so the application can relaunch or recover instead of
// being terminated. Returning false falls through to the stock behaviour.
//
// Pass nil to restore stock behaviour.
func SetWebView2ProcessFailedRecoveryHook(hook func(kind WebView2ProcessFailedKind) bool) {
	if hook == nil {
		edge.ProcessFailedRecoveryHook = nil
		return
	}
	edge.ProcessFailedRecoveryHook = func(kind edge.COREWEBVIEW2_PROCESS_FAILED_KIND) bool {
		return hook(WebView2ProcessFailedKind(kind))
	}
}

// SetWebView2MemoryDiagnosticsHook installs a function whose one-line memory
// summary is appended to the log line WebView2 writes when controller creation
// fails, often the last line written before an out-of-memory kill. Pass nil to
// remove it.
func SetWebView2MemoryDiagnosticsHook(fn func() string) {
	edge.MemoryDiagnostics = fn
}

// SetWebView2MemoryUsageTargetLow lowers (true) or restores (false) the
// window's WebView2 memory target, through ICoreWebView2_19's
// MemoryUsageTargetLevel. While LOW, WebView2 trims caches and releases what
// it can, which suits a window that is hidden or minimized; script, timers,
// audio and WebRTC keep running. Restore NORMAL when the window is shown
// again.
//
// Asynchronous and best-effort: it is dispatched to the UI thread, and on a
// runtime older than the interface (114 and earlier), or a window not yet
// created or already destroyed, it does nothing.
func (w *WebviewWindow) SetWebView2MemoryUsageTargetLow(low bool) {
	if w == nil || w.impl == nil || w.isDestroyed() {
		return
	}
	InvokeAsync(func() {
		if w.isDestroyed() {
			return
		}
		impl, ok := w.impl.(*windowsWebviewWindow)
		if !ok || impl == nil || impl.chromium == nil {
			return
		}
		level := edge.COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_NORMAL
		if low {
			level = edge.COREWEBVIEW2_MEMORY_USAGE_TARGET_LEVEL_LOW
		}
		// Logged, not routed through App.error: that reaches the
		// application's ErrorHandler, which may treat WebView2 errors as
		// fatal, and a failed memory hint never is.
		if err := impl.chromium.SetMemoryUsageTargetLevel(level); err != nil && err != edge.UnsupportedCapabilityError {
			log.Printf("[WebView2] MemoryUsageTargetLevel(low=%v) failed for window %d: %v", low, w.id, err)
		}
	})
}

// init wires WebView2's transient-failure retries into the main-thread
// dispatcher.
//
// ExecuteScript and PostWebMessageAsString must run on the thread that owns the
// WebView2 controller, which is the main thread (execJS dispatches onto it, and
// PostWebMessageAsString runs inside a COM event callback). A retry therefore
// cannot sleep in place without blocking the message pump. time.AfterFunc waits
// on its own goroutine and dispatchOnMainThread hands the attempt back to the
// right thread, so the UI keeps running while WebView2 recovers.
//
// Retried calls can land after calls issued later, since only the failing one
// is delayed. That is deliberate: reordering a fire-and-forget script during a
// GPU reset is a far smaller problem than freezing the UI for seconds, which is
// long enough to trip an application freeze watchdog.
func init() {
	edge.SetRetryScheduler(func(delay time.Duration, fn func()) {
		time.AfterFunc(delay, func() {
			app := globalApplication
			if app == nil {
				return
			}
			app.dispatchOnMainThread(fn)
		})
	})
}
