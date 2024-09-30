package main

import (
	"image"
	"unsafe"
)

type Area struct {
	AreaID uint8

	Width  uint16 // width in 8x8 tiles
	Height uint16 // height in 8x8 tiles

	IsLoaded bool

	Rendered      image.Image
	RenderedNRGBA *image.NRGBA

	TilesVisited map[MapCoord]empty

	Map8          [0x4000]uint16 // for presentation
	Tiles         [0x4000]byte   // for tile types
	Reachable     [0x4000]byte
	Hookshot      map[MapCoord]byte
	AllowDirFlags [0x4000]uint8

	e               *System
	WRAM            [0x20000]byte
	WRAMAfterLoaded [0x20000]byte
	VRAMTileSet     [0x4000]byte

	Entrances []AreaEntrance
}

type AreaEntrance struct {
	TileIndex  uint16
	EntranceID uint8
}

func (a *Area) Traverse(c OWCoord, d Direction, inc int) (OWCoord, Direction, bool) {
	it := int(c)
	row := (it & 0xF80) >> 7
	col := it & 0x7F

	// don't allow perpendicular movement along the outer edge; this prevents accidental/leaky flood fill
	if (col <= 1 || col >= int(a.Width-1)) && (d != DirEast && d != DirWest) {
		return c, d, false
	} else if (row <= 1 || row >= int(a.Height)-1) && (d != DirNorth && d != DirSouth) {
		return c, d, false
	}

	switch d {
	case DirNorth:
		if row >= 0+inc {
			return OWCoord(it - (inc << 7)), d, true
		}
		return c, d, false
	case DirSouth:
		if row <= int(a.Height)-inc {
			return OWCoord(it + (inc << 7)), d, true
		}
		return c, d, false
	case DirWest:
		if col >= 0+inc {
			return OWCoord(it - inc), d, true
		}
		return c, d, false
	case DirEast:
		if col <= int(a.Width)-inc {
			return OWCoord(it + inc), d, true
		}
		return c, d, false
	default:
		panic("bad direction")
	}
}

func (a *Area) Render() {
	cgram := (*(*[0x100]uint16)(unsafe.Pointer(&a.WRAM[0xC300])))[:]
	pal := cgramToPalette(cgram)
	bg1 := [2]*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, int(a.Width)*8, int(a.Height)*8), pal),
		image.NewPaletted(image.Rect(0, 0, int(a.Width)*8, int(a.Height)*8), pal),
	}
	bg2 := [2]*image.Paletted{
		image.NewPaletted(image.Rectangle{}, nil),
		image.NewPaletted(image.Rectangle{}, nil),
	}
	renderMap8(bg1, int(a.Width), int(a.Height), a.Map8[:], a.VRAMTileSet[:], drawBG1p0, drawBG1p1)

	// compose the priority layers:
	g := image.NewNRGBA(image.Rect(0, 0, int(a.Width)*8, int(a.Height)*8))
	ComposeToNonPalettedOW(g, pal, bg1, bg2, int(a.Width), false, false)
	// renderSpriteLabels(g, wram, Supertile(read16(wram, 0xA0)))
	// exportPNG("a-test-bg1p0.png", bg1[0])
	// exportPNG("a-test-bg1p1.png", bg1[1])

	a.RenderedNRGBA = g
	a.Rendered = g
}
