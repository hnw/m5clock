package main

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/const2bit"
)

type ColorFont struct {
	Base         *const2bit.Font
	Background   color.RGBA
	ColonYOffset int8

	glyph ColorGlyph
}

type ColorGlyph struct {
	Rune       rune
	Width      uint8
	Height     uint8
	XAdvance   uint8
	XOffset    int8
	YOffset    int8
	Bitmaps    []byte
	Background color.RGBA
}

func (g *ColorGlyph) Info() tinyfont.GlyphInfo {
	return tinyfont.GlyphInfo{
		Rune:     g.Rune,
		Width:    g.Width,
		Height:   g.Height,
		XAdvance: g.XAdvance,
		XOffset:  g.XOffset,
		YOffset:  g.YOffset,
	}
}

func blend(bg, fg color.RGBA, alpha uint8) color.RGBA {
	a := uint16(alpha)
	ia := uint16(255 - alpha)

	return color.RGBA{
		R: blendChannel(uint16(bg.R), uint16(fg.R), ia, a),
		G: blendChannel(uint16(bg.G), uint16(fg.G), ia, a),
		B: blendChannel(uint16(bg.B), uint16(fg.B), ia, a),
		A: 255,
	}
}

// blendChannel mixes one colour channel. Both weights are complementary
// multiples of 255, so the weighted average always lands back in 0..255; the
// guard is only there to keep that invariant explicit to the reader and to
// the analyser.
func blendChannel(bg, fg, ia, a uint16) uint8 {
	v := (bg*ia + fg*a) / 255
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func (g *ColorGlyph) Draw(
	display drivers.Displayer,
	x int16,
	y int16,
	foreground color.RGBA,
) {
	offset := 0
	var bitmap byte

	if len(g.Bitmaps) != 0 {
		bitmap = g.Bitmaps[0]
	}

	bit := uint8(0)

	for j := int16(0); j < int16(g.Height); j++ {
		for i := int16(0); i < int16(g.Width); i++ {

			var alpha uint8

			switch bitmap & 0xC0 {
			case 0xC0:
				alpha = 255
			case 0x80:
				alpha = 127
			case 0x40:
				alpha = 47
			default:
				alpha = 0
			}

			if alpha != 0 {
				c := blend(g.Background, foreground, alpha)

				display.SetPixel(
					x+int16(g.XOffset)+i,
					y+int16(g.YOffset)+j,
					c,
				)
			}

			bitmap <<= 2
			bit += 2

			if bit > 7 {
				offset++
				if offset < len(g.Bitmaps) {
					bitmap = g.Bitmaps[offset]
				}
				bit = 0
			}
		}
	}
}

func (f *ColorFont) GetYAdvance() uint8 {
	return f.Base.YAdvance
}

// signedOffset reinterprets one byte of glyph metrics as the signed value it
// encodes. tinyfontgen stores negative offsets in two's complement, so bytes
// from 0x80 upwards mean -128..-1.
func signedOffset(v uint8) int8 {
	if v < 0x80 {
		return int8(v)
	}
	signed := int(v) - 0x100
	if signed < -0x80 {
		signed = -0x80
	}
	return int8(signed)
}

func (f *ColorFont) GetGlyph(r rune) tinyfont.Glypher {
	offsetMap := f.Base.OffsetMap
	data := f.Base.Data

	s := 0
	e := len(offsetMap)/6 - 1

	for s <= e {
		m := (s + e) / 2

		r2 := rune(offsetMap[m*6])<<16 +
			rune(offsetMap[m*6+1])<<8 +
			rune(offsetMap[m*6+2])

		if r2 < r {
			s = m + 1
		} else {
			e = m - 1
		}
	}

	if s > len(offsetMap)/6-1 {
		s = 0
	}

	offset := int(offsetMap[s*6+3])<<16 +
		int(offsetMap[s*6+4])<<8 +
		int(offsetMap[s*6+5])

	size := len(data[offset+5:])

	if s*6+6 < len(offsetMap) {
		size =
			int(offsetMap[s*6+9])<<16 +
				int(offsetMap[s*6+10])<<8 +
				int(offsetMap[s*6+11]) -
				offset
	}

	f.glyph.Rune = r
	f.glyph.Width = data[offset]
	f.glyph.Height = data[offset+1]
	f.glyph.XAdvance = data[offset+2]
	f.glyph.XOffset = signedOffset(data[offset+3])
	f.glyph.YOffset = signedOffset(data[offset+4])
	f.glyph.Bitmaps = []byte(data[offset+5 : offset+5+size])
	f.glyph.Background = f.Background

	if r == ':' {
		f.glyph.YOffset += f.ColonYOffset
	}

	return &f.glyph
}
