package main

import "time"

// runUI owns the display, renderer (and therefore font instances), and the
// previous frame. Other goroutines communicate with it only through inbox.
// initialLocation comes from the build-time zone specification and stays
// fixed for the lifetime of the process.
//
// Only main_m5stack.go calls it, so the host build sees no reference.
//
//nolint:unused
func runUI(display ClockDisplay, initialLocation *time.Location, inbox <-chan UIMessage) {
	renderer := newClockRenderer(display)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	app := AppState{Location: initialLocation}
	var previous ViewState
	first := true

	for {
		force := false
		select {
		case msg, ok := <-inbox:
			if !ok {
				inbox = nil
				continue
			}
			app = applyMessage(app, msg)
			force = msg.Kind == UIMessageForceRedraw
		case <-ticker.C:
		}

		current := buildViewState(time.Now(), app)
		dirty := diffViewState(previous, current, first || force)
		renderer.renderClock(current, dirty)
		previous = current
		first = false
	}
}
