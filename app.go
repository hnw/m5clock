// Package main renders a clock and a Japanese date on an M5Stack Fire LCD
// and keeps the clock synchronised over the network.
package main

import (
	"errors"
	"time"
)

// SyncState is the small piece of network state needed by the clock UI.
type SyncState uint8

const (
	SyncUnknown SyncState = iota
	Syncing
	Synced
	SyncFailed
)

// ScreenID identifies a top-level UI screen. Only the clock is rendered today;
// ScreenBlank provides a useful target for future button-driven transitions.
type ScreenID uint8

const (
	ScreenClock ScreenID = iota
	ScreenBlank
)

// UIMessageKind describes a command or external state transition for the UI.
type UIMessageKind uint8

const (
	UIMessageSyncChanged UIMessageKind = iota
	UIMessageSetScreen
	UIMessageForceRedraw
)

// UIMessage is the typed inbox payload shared by UI event producers.
type UIMessage struct {
	Kind   UIMessageKind
	Sync   SyncState
	Screen ScreenID
}

// AppState contains state that is not derived from the current time.
// Location is seeded from the build-time zone specification and never
// changes afterwards, so no mutex is needed around it.
type AppState struct {
	Sync     SyncState
	Location *time.Location
	Screen   ScreenID
}

// ViewState is a fixed snapshot consumed by the renderer. Rendering never
// reads the clock itself, so one frame cannot straddle a second boundary.
type ViewState struct {
	Date   string
	Clock  string
	Second string
	Sync   SyncState
	Screen ScreenID
}

// DirtyFlags identifies independently redrawable regions.
type DirtyFlags uint8

const DirtyNone DirtyFlags = 0

const (
	DirtyDate DirtyFlags = 1 << iota
	DirtyClock
	DirtySecond
	DirtyStatus
	// DirtyAll is deliberately a separate bit so it cannot collide with a
	// combination of ordinary dirty regions.
	DirtyAll
)

// applyMessage updates application state without performing I/O.
func applyMessage(state AppState, msg UIMessage) AppState {
	switch msg.Kind {
	case UIMessageSyncChanged:
		state.Sync = msg.Sync
	case UIMessageSetScreen:
		state.Screen = msg.Screen
	case UIMessageForceRedraw:
		// A force redraw is a command and does not change persistent state.
	}
	return state
}

// buildViewState derives all display text from one captured point in time.
func buildViewState(now time.Time, app AppState) ViewState {
	if app.Location != nil {
		now = now.In(app.Location)
	}
	return ViewState{
		Date:   formatDate(now),
		Clock:  formatClock(now.Hour(), now.Minute()),
		Second: formatSecond(now.Second()),
		Sync:   app.Sync,
		Screen: app.Screen,
	}
}

// diffViewState determines the smallest set of regions that must be redrawn.
func diffViewState(previous, current ViewState, force bool) DirtyFlags {
	if force || previous.Screen != current.Screen {
		return DirtyAll
	}

	var dirty DirtyFlags
	if previous.Date != current.Date {
		dirty |= DirtyDate | DirtyClock | DirtySecond
	} else if previous.Clock != current.Clock {
		dirty |= DirtyClock | DirtySecond
	} else if previous.Second != current.Second {
		dirty |= DirtySecond
	}
	if previous.Sync != current.Sync {
		dirty |= DirtyStatus
	}
	return dirty
}

func formatClock(hour, minute int) string {
	b := [5]byte{
		digit(hour / 10),
		digit(hour % 10),
		':',
		digit(minute / 10),
		digit(minute % 10),
	}
	return string(b[:])
}

func formatSecond(second int) string {
	b := [2]byte{
		digit(second / 10),
		digit(second % 10),
	}
	return string(b[:])
}

func formatDate(now time.Time) string {
	var b [18]byte
	n := appendOneOrTwoDigits(b[:], 0, int(now.Month()))
	n += copy(b[n:], "月")
	n = appendOneOrTwoDigits(b[:], n, now.Day())
	n += copy(b[n:], "日(")
	n += copy(b[n:], japaneseWeekday(now.Weekday()))
	b[n] = ')'
	n++
	return string(b[:n])
}

// digit renders a single decimal digit as ASCII. Callers only ever pass
// clock components that are already in range; anything else degrades to '0'
// so a bad value can never turn into a stray control character.
func digit(value int) byte {
	if value < 0 || value > 9 {
		return '0'
	}
	return byte('0' + value)
}

func appendOneOrTwoDigits(dst []byte, at, value int) int {
	if value >= 10 {
		dst[at] = digit(value / 10)
		dst[at+1] = digit(value % 10)
		return at + 2
	}
	dst[at] = digit(value)
	return at + 1
}

func japaneseWeekday(day time.Weekday) string {
	switch day {
	case time.Sunday:
		return "日"
	case time.Monday:
		return "月"
	case time.Tuesday:
		return "火"
	case time.Wednesday:
		return "水"
	case time.Thursday:
		return "木"
	case time.Friday:
		return "金"
	case time.Saturday:
		return "土"
	default:
		return "日"
	}
}

// timezone is injected at build time with -ldflags so one source tree can
// produce binaries for different regions without embedding credentials or
// zone databases in the repository:
//
//	-ldflags "-X main.timezone=+09:00"
//
// See parseTimeZoneSpec for the accepted forms.
//
// Only the m5stack build reads it, so the host build sees no reference.
//
//nolint:unused
var timezone string

var errTimeZoneSpec = errors.New("time zone spec must be empty (UTC) or +HH:MM / -HH:MM, e.g. +09:00")

// parseTimeZoneSpec converts a build-time zone specification into a fixed
// *time.Location. The spec is either empty (UTC) or an exact ±HH:MM UTC
// offset such as "+09:00" or "-05:30". Real-world zone offsets are all
// minute-granular, so this covers every IANA zone; named zones and DST
// tracking are deliberate non-goals. A wrong specification must not brick
// the clock, so callers are expected to fall back to UTC on error.
func parseTimeZoneSpec(spec string) (*time.Location, error) {
	if spec == "" {
		return time.UTC, nil
	}
	if len(spec) != 6 || spec[3] != ':' ||
		(spec[0] != '+' && spec[0] != '-') {
		return nil, errTimeZoneSpec
	}
	hours, err := parseTimeZoneDigits(spec[1:3])
	if err != nil {
		return nil, err
	}
	minutes, err := parseTimeZoneDigits(spec[4:6])
	if err != nil {
		return nil, err
	}
	if hours > 23 || minutes > 59 {
		return nil, errTimeZoneSpec
	}

	offset := hours*3600 + minutes*60
	if spec[0] == '-' {
		offset = -offset
	}
	return time.FixedZone(spec, offset), nil
}

// parseTimeZoneDigits parses exactly two ASCII digits, without pulling in
// strconv.
func parseTimeZoneDigits(digits string) (int, error) {
	if (digits[0] < '0' || digits[0] > '9') || (digits[1] < '0' || digits[1] > '9') {
		return 0, errTimeZoneSpec
	}
	return int(digits[0]-'0')*10 + int(digits[1]-'0'), nil
}
