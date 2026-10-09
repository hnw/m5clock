package main

import (
	"testing"
	"time"
)

func TestApplyMessage(t *testing.T) {
	tests := []struct {
		name string
		from AppState
		msg  UIMessage
		want AppState
	}{
		{
			name: "syncing",
			from: AppState{Sync: Synced, Screen: ScreenBlank},
			msg:  UIMessage{Kind: UIMessageSyncChanged, Sync: Syncing},
			want: AppState{Sync: Syncing, Screen: ScreenBlank},
		},
		{
			name: "synced",
			from: AppState{Sync: Syncing, Screen: ScreenClock},
			msg:  UIMessage{Kind: UIMessageSyncChanged, Sync: Synced},
			want: AppState{Sync: Synced, Screen: ScreenClock},
		},
		{
			name: "failed",
			from: AppState{Sync: Syncing, Screen: ScreenClock},
			msg:  UIMessage{Kind: UIMessageSyncChanged, Sync: SyncFailed},
			want: AppState{Sync: SyncFailed, Screen: ScreenClock},
		},
		{
			name: "screen",
			from: AppState{Sync: Synced, Screen: ScreenClock},
			msg:  UIMessage{Kind: UIMessageSetScreen, Screen: ScreenBlank},
			want: AppState{Sync: Synced, Screen: ScreenBlank},
		},
		{
			name: "force redraw preserves state",
			from: AppState{Sync: SyncFailed, Screen: ScreenBlank},
			msg:  UIMessage{Kind: UIMessageForceRedraw},
			want: AppState{Sync: SyncFailed, Screen: ScreenBlank},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applyMessage(test.from, test.msg); got != test.want {
				t.Fatalf("applyMessage() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestBuildViewStateNormalTime(t *testing.T) {
	now := time.Date(2026, 8, 13, 18, 42, 3, 0, time.FixedZone("JST", 9*60*60))
	got := buildViewState(now, AppState{Sync: Synced, Screen: ScreenClock})
	want := ViewState{
		Date:   "8月13日(木)",
		Clock:  "18:42",
		Second: "03",
		Sync:   Synced,
		Screen: ScreenClock,
	}
	if got != want {
		t.Fatalf("buildViewState() = %#v, want %#v", got, want)
	}
}

func TestBuildViewStateCalendarBoundaries(t *testing.T) {
	location := time.FixedZone("JST", 9*60*60)
	tests := []struct {
		name       string
		now        time.Time
		wantDate   string
		wantClock  string
		wantSecond string
	}{
		{
			name:       "end of day",
			now:        time.Date(2026, 8, 13, 23, 59, 59, 0, location),
			wantDate:   "8月13日(木)",
			wantClock:  "23:59",
			wantSecond: "59",
		},
		{
			name:       "next day",
			now:        time.Date(2026, 8, 14, 0, 0, 0, 0, location),
			wantDate:   "8月14日(金)",
			wantClock:  "00:00",
			wantSecond: "00",
		},
		{
			name:       "month end",
			now:        time.Date(2025, 1, 31, 23, 59, 59, 0, location),
			wantDate:   "1月31日(金)",
			wantClock:  "23:59",
			wantSecond: "59",
		},
		{
			name:       "next month",
			now:        time.Date(2025, 2, 1, 0, 0, 0, 0, location),
			wantDate:   "2月1日(土)",
			wantClock:  "00:00",
			wantSecond: "00",
		},
		{
			name:       "year end",
			now:        time.Date(2025, 12, 31, 23, 59, 59, 0, location),
			wantDate:   "12月31日(水)",
			wantClock:  "23:59",
			wantSecond: "59",
		},
		{
			name:       "new year",
			now:        time.Date(2026, 1, 1, 0, 0, 0, 0, location),
			wantDate:   "1月1日(木)",
			wantClock:  "00:00",
			wantSecond: "00",
		},
		{
			name:       "leap year february 28",
			now:        time.Date(2024, 2, 28, 23, 59, 59, 0, location),
			wantDate:   "2月28日(水)",
			wantClock:  "23:59",
			wantSecond: "59",
		},
		{
			name:       "leap day",
			now:        time.Date(2024, 2, 29, 0, 0, 0, 0, location),
			wantDate:   "2月29日(木)",
			wantClock:  "00:00",
			wantSecond: "00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := buildViewState(test.now, AppState{})
			if got.Date != test.wantDate || got.Clock != test.wantClock || got.Second != test.wantSecond {
				t.Fatalf("buildViewState() = %q %q:%q, want %q %q:%q",
					got.Date, got.Clock, got.Second,
					test.wantDate, test.wantClock, test.wantSecond)
			}
		})
	}
}

func TestBuildViewStateUsesProvidedTimeZone(t *testing.T) {
	instant := time.Date(2026, 8, 13, 15, 30, 0, 0, time.UTC)
	jst := time.FixedZone("JST", 9*60*60)
	got := buildViewState(instant, AppState{Location: jst})
	if got.Date != "8月14日(金)" || got.Clock != "00:30" {
		t.Fatalf("JST view = %q %q, want %q %q", got.Date, got.Clock, "8月14日(金)", "00:30")
	}
}

func TestParseTimeZoneSpec(t *testing.T) {
	tests := []struct {
		name           string
		spec           string
		wantName       string
		wantOffsetSecs int
	}{
		{name: "empty defaults to UTC", spec: "", wantName: "UTC", wantOffsetSecs: 0},
		{name: "tokyo", spec: "+09:00", wantName: "+09:00", wantOffsetSecs: 9 * 3600},
		{name: "half hour offset", spec: "+05:30", wantName: "+05:30", wantOffsetSecs: 5*3600 + 30*60},
		{name: "quarter hour offset", spec: "+05:45", wantName: "+05:45", wantOffsetSecs: 5*3600 + 45*60},
		{name: "negative", spec: "-08:00", wantName: "-08:00", wantOffsetSecs: -8 * 3600},
		{name: "negative half hour", spec: "-09:30", wantName: "-09:30", wantOffsetSecs: -(9*3600 + 30*60)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			location, err := parseTimeZoneSpec(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			if location.String() != test.wantName {
				t.Fatalf("name = %q, want %q", location.String(), test.wantName)
			}
			_, offset := time.Date(2026, 1, 1, 0, 0, 0, 0, location).Zone()
			if offset != test.wantOffsetSecs {
				t.Fatalf("offset = %d seconds, want %d", offset, test.wantOffsetSecs)
			}
		})
	}
}

func TestParseTimeZoneSpecErrors(t *testing.T) {
	for _, spec := range []string{
		"9:00",
		"09:00",
		"+9:00",
		"+0900",
		"+09:0",
		"+09:000",
		"+0:60",
		"+24:00",
		"+09:60",
		"+0a:00",
		"+09:0a",
		"+09:00 ",
		" Asia/Tokyo",
		"Asia/Tokyo",
		"Asia/Tokyo@+09:00",
		"utc",
		"-",
	} {
		if location, err := parseTimeZoneSpec(spec); err == nil {
			t.Fatalf("parseTimeZoneSpec(%q) = %v, want error", spec, location)
		}
	}
}

func TestDiffViewState(t *testing.T) {
	base := ViewState{
		Date: "8月13日(木)", Clock: "18:42", Second: "03",
		Sync: Synced, Screen: ScreenClock,
	}

	tests := []struct {
		name    string
		current ViewState
		force   bool
		want    DirtyFlags
	}{
		{name: "same", current: base, want: DirtyNone},
		{name: "second", current: withSecond(base, "04"), want: DirtySecond},
		{name: "minute", current: withClockAndSecond(base, "18:43", "00"), want: DirtyClock | DirtySecond},
		{name: "hour", current: withClockAndSecond(base, "19:00", "00"), want: DirtyClock | DirtySecond},
		{name: "date", current: withDateClockAndSecond(base, "8月14日(金)", "00:00", "00"), want: DirtyDate | DirtyClock | DirtySecond},
		{name: "sync", current: withSync(base, SyncFailed), want: DirtyStatus},
		{name: "force", current: base, force: true, want: DirtyAll},
		{name: "screen", current: withScreen(base, ScreenBlank), want: DirtyAll},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := diffViewState(base, test.current, test.force); got != test.want {
				t.Fatalf("diffViewState() = %08b, want %08b", got, test.want)
			}
		})
	}

	if got := diffViewState(ViewState{}, base, true); got != DirtyAll {
		t.Fatalf("initial diff = %08b, want DirtyAll", got)
	}
}

func withSecond(v ViewState, second string) ViewState {
	v.Second = second
	return v
}

func withClockAndSecond(v ViewState, clock, second string) ViewState {
	v.Clock = clock
	v.Second = second
	return v
}

func withDateClockAndSecond(v ViewState, date, clock, second string) ViewState {
	v.Date = date
	v.Clock = clock
	v.Second = second
	return v
}

func withSync(v ViewState, sync SyncState) ViewState {
	v.Sync = sync
	return v
}

func withScreen(v ViewState, screen ScreenID) ViewState {
	v.Screen = screen
	return v
}
