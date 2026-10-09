package main

import (
	"image/color"
	"testing"
	"time"
)

type recordingDisplay struct {
	width, height int16
	outOfBounds   int
	setPixels     int
	displayCalls  int
}

func newRecordingDisplay(width, height int16) *recordingDisplay {
	return &recordingDisplay{width: width, height: height}
}

func (d *recordingDisplay) Size() (int16, int16) {
	return d.width, d.height
}

func (d *recordingDisplay) SetPixel(x, y int16, _ color.RGBA) {
	if x < 0 || y < 0 || x >= d.width || y >= d.height {
		d.outOfBounds++
		return
	}
	d.setPixels++
}

func (d *recordingDisplay) Display() error {
	d.displayCalls++
	return nil
}

func (d *recordingDisplay) FillRectangle(x, y, width, height int16, _ color.RGBA) error {
	if x < 0 || y < 0 || width <= 0 || height <= 0 || x+width > d.width || y+height > d.height {
		d.outOfBounds++
		return nil
	}
	d.setPixels += int(width) * int(height)
	return nil
}

func TestRenderClockStaysInsideDisplay(t *testing.T) {
	display := newRecordingDisplay(displayWidth, displayHeight)
	renderer := newClockRenderer(display)
	now := time.Date(2026, 8, 13, 18, 42, 3, 0, time.UTC)
	renderer.renderClock(buildViewState(now, AppState{Sync: Synced}), DirtyAll)

	if display.outOfBounds != 0 {
		t.Fatalf("renderer attempted %d out-of-bounds draws", display.outOfBounds)
	}
	if display.setPixels == 0 {
		t.Fatal("full render drew no pixels")
	}
	if display.displayCalls != 1 {
		t.Fatalf("Display calls = %d, want 1", display.displayCalls)
	}
}

func TestRenderClockLongestDateStaysInsideDisplay(t *testing.T) {
	display := newRecordingDisplay(displayWidth, displayHeight)
	renderer := newClockRenderer(display)
	state := ViewState{
		Date: "12月31日(水)", Clock: "23:59", Second: "59",
		Sync: SyncFailed, Screen: ScreenClock,
	}
	renderer.renderClock(state, DirtyAll)
	if display.outOfBounds != 0 {
		t.Fatalf("longest date attempted %d out-of-bounds draws", display.outOfBounds)
	}
}

func TestRenderSecondOnlyStaysInsideDisplay(t *testing.T) {
	display := newRecordingDisplay(displayWidth, displayHeight)
	renderer := newClockRenderer(display)
	state := ViewState{Date: "8月13日(木)", Clock: "18:42", Second: "04", Screen: ScreenClock}
	renderer.renderClock(state, DirtySecond)
	if display.outOfBounds != 0 {
		t.Fatalf("second render attempted %d out-of-bounds draws", display.outOfBounds)
	}
	if display.setPixels == 0 {
		t.Fatal("second-only render drew no pixels")
	}
}

func TestBlend(t *testing.T) {
	background := color.RGBA{R: 10, G: 20, B: 30, A: 255}
	foreground := color.RGBA{R: 210, G: 120, B: 60, A: 255}

	if got := blend(background, foreground, 0); got != background {
		t.Fatalf("background blend = %#v, want %#v", got, background)
	}
	if got := blend(background, foreground, 255); got != foreground {
		t.Fatalf("foreground blend = %#v, want %#v", got, foreground)
	}
	wantMiddle := color.RGBA{
		R: middleChannel(background.R, foreground.R),
		G: middleChannel(background.G, foreground.G),
		B: middleChannel(background.B, foreground.B),
		A: 255,
	}
	if got := blend(background, foreground, 127); got != wantMiddle {
		t.Fatalf("middle blend = %#v, want %#v", got, wantMiddle)
	}
}

// middleChannel is an independent reimplementation of the expected result so
// the test does not lean on the helpers it is checking.
func middleChannel(bg, fg uint8) uint8 {
	v := (uint16(bg)*128 + uint16(fg)*127) / 255
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func TestColorFontColonYOffset(t *testing.T) {
	baseInfo := InterClock105.GetGlyph(':').Info()
	font := ColorFont{Base: &InterClock105, ColonYOffset: -13}
	got := font.GetGlyph(':').Info()
	if got.YOffset != baseInfo.YOffset-13 {
		t.Fatalf("colon Y offset = %d, want %d", got.YOffset, baseInfo.YOffset-13)
	}
}

func TestColorGlyphDrawStaysInsideDisplay(t *testing.T) {
	display := newRecordingDisplay(2, 2)
	glyph := ColorGlyph{
		Width: 2, Height: 2, XAdvance: 2,
		Bitmaps: []byte{0xff}, Background: colorBlack,
	}
	glyph.Draw(display, 0, 0, colorWhite)
	if display.outOfBounds != 0 {
		t.Fatalf("glyph attempted %d out-of-bounds draws", display.outOfBounds)
	}
	if display.setPixels != 4 {
		t.Fatalf("glyph drew %d pixels, want 4", display.setPixels)
	}
}

func TestGeneratedFontsContainRequiredRunes(t *testing.T) {
	for _, r := range "0123456789月火水木金土日()" {
		if !offsetMapContains(JapaneseDate32.OffsetMap, r) {
			t.Errorf("JapaneseDate32 is missing %q", r)
		}
	}
	for _, r := range "0123456789:" {
		if !offsetMapContains(InterClock105.OffsetMap, r) {
			t.Errorf("InterClock105 is missing %q", r)
		}
	}
}

func offsetMapContains(offsetMap string, want rune) bool {
	for i := 0; i+5 < len(offsetMap); i += 6 {
		got := rune(offsetMap[i])<<16 | rune(offsetMap[i+1])<<8 | rune(offsetMap[i+2])
		if got == want {
			return true
		}
	}
	return false
}
