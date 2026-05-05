package application

type EventIPCTransport struct {
	app *App
}

func (t *EventIPCTransport) DispatchWailsEvent(event *CustomEvent) {
	// Snapshot windows under RLock, then release before dispatching.
	// Holding the read lock across DispatchWailsEvent (which calls ExecJS →
	// InvokeSync to the GTK main thread on Linux) deadlocks if a writer
	// queues up — Go's RWMutex blocks new readers behind a queued writer,
	// so the GTK thread's own RLock (e.g. inside onProcessRequest) stalls
	// while the active reader waits for the GTK thread to drain its queue.
	t.app.windowsLock.RLock()
	windows := make([]Window, 0, len(t.app.windows))
	for _, window := range t.app.windows {
		windows = append(windows, window)
	}
	t.app.windowsLock.RUnlock()

	for _, window := range windows {
		if event.IsCancelled() {
			return
		}
		window.DispatchWailsEvent(event)
	}
}
