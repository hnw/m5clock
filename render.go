package main

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
)

const (
	displayWidth  int16 = 320
	displayHeight int16 = 240
)

var (
	colorBlack     = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	colorWhite     = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colorClockGray = color.RGBA{R: 230, G: 230, B: 230, A: 255}
)

// ClockDisplay is the smallest display surface needed by the common renderer.
type ClockDisplay interface {
	drivers.Displayer
	FillRectangle(x, y, width, height int16, c color.RGBA) error
}

type clockRenderer struct {
	display   ClockDisplay
	dateFont  ColorFont
	clockFont ColorFont
}

func newClockRenderer(display ClockDisplay) *clockRenderer {
	return &clockRenderer{
		display: display,
		dateFont: ColorFont{
			Base:       &JapaneseDate32,
			Background: colorBlack,
		},
		clockFont: ColorFont{
			Base:         &InterClock105,
			Background:   colorBlack,
			ColonYOffset: -13,
		},
	}
}

func (r *clockRenderer) renderClock(state ViewState, dirty DirtyFlags) {
	if dirty == DirtyNone {
		return
	}

	if hasDirty(dirty, DirtyAll) {
		r.fill(0, 0, displayWidth, displayHeight, colorBlack)
	}

	if state.Screen != ScreenClock {
		_ = r.display.Display()
		return
	}

	if hasDirty(dirty, DirtyDate) {
		r.fill(0, 0, 288, 70, colorBlack)
		x := centeredTextX(&r.dateFont, state.Date)
		tinyfont.WriteLine(r.display, &r.dateFont, x, 54, state.Date, colorWhite)
	}

	if hasDirty(dirty, DirtyClock) {
		r.fill(0, 70, displayWidth, 125, colorBlack)
		x := centeredTextX(&r.clockFont, state.Clock)
		tinyfont.WriteLine(r.display, &r.clockFont, x, 175, state.Clock, colorClockGray)
	}

	if hasDirty(dirty, DirtySecond) {
		r.fill(0, 195, displayWidth, 45, colorBlack)
		x := centeredTextX(&r.dateFont, state.Second)
		tinyfont.WriteLine(r.display, &r.dateFont, x, 229, state.Second, colorWhite)
	}

	if hasDirty(dirty, DirtyStatus) {
		r.drawSyncStatus(state.Sync)
	}

	_ = r.display.Display()
}

func centeredTextX(font tinyfont.Fonter, text string) int16 {
	_, outer := tinyfont.LineWidth(font, text)
	if outer >= uint32(displayWidth) {
		return 0
	}
	return (displayWidth - int16(outer)) / 2
}

func hasDirty(dirty, region DirtyFlags) bool {
	return dirty&DirtyAll != 0 || dirty&region != 0
}

func (r *clockRenderer) drawSyncStatus(state SyncState) {
	r.fill(288, 0, 32, 32, colorBlack)
	c := color.RGBA{R: 80, G: 80, B: 80, A: 255}
	switch state {
	case Syncing:
		c = color.RGBA{R: 255, G: 180, B: 48, A: 255}
	case Synced:
		c = color.RGBA{R: 64, G: 210, B: 110, A: 255}
	case SyncFailed:
		c = color.RGBA{R: 240, G: 70, B: 70, A: 255}
	}
	r.fill(300, 11, 9, 9, c)
}

func (r *clockRenderer) fill(x, y, width, height int16, c color.RGBA) {
	if err := r.display.FillRectangle(x, y, width, height, c); err != nil {
		println("display fill failed")
	}
}
