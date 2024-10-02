package main

import (
	"image"
	"image/color"
	"unsafe"

	"golang.org/x/image/draw"
)

type Area struct {
	AreaID uint8

	Width  uint16 // width in 8x8 tiles
	Height uint16 // height in 8x8 tiles

	IsLoaded bool

	// image rendering:
	Rendered      image.Image
	RenderedNRGBA *image.NRGBA

	// map data:
	Map8  [0x4000]uint16 // for presentation
	Tiles [0x4000]byte   // for tile types

	// flood fill state:
	TilesVisited  map[OWCoord]empty
	Reachable     [0x4000]byte
	Hookshot      [0x4000]uint8
	AllowDirFlags [0x4000]uint8

	e               *System
	WRAM            [0x20000]byte
	WRAMAfterLoaded [0x20000]byte
	VRAMTileSet     [0x4000]byte

	Entrances []AreaEntrance
}

type AreaEntrance struct {
	OWCoord    OWCoord
	EntranceID uint8
}

func (a *Area) RowCol(c OWCoord) (row, col int) {
	row = int((c & 0x3F80) >> 7)
	col = int(c & 0x7F)
	return
}

func (a *Area) Traverse(c OWCoord, d Direction, inc int) (OWCoord, Direction, bool) {
	it := int(c)
	row, col := a.RowCol(c)

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

// isAlwaysWalkable checks if the tile is always walkable on, regardless of state
func (a *Area) isAlwaysWalkable(v uint8) bool {
	return v == 0x00 || // no collision
		v == 0x09 || // shallow water
		v == 0x22 || // manual stairs
		v == 0x23 || v == 0x24 || // floor switches
		(v >= 0x0D && v <= 0x0F) || // spikes / floor ice
		v == 0x3A || v == 0x3B || // star tiles
		v == 0x40 || // thick grass
		v == 0x48 || // diggable ground
		v == 0x4B || // warp
		v == 0x60 || // rupee tile
		(v >= 0x68 && v <= 0x6B) || // conveyors
		v == 0xA0 || // north/south dungeon swap door (for HC to sewers)
		v&0xF0 == 0x70 || // pots/pegs/blocks
		v == 0x62 || // bombable floor
		v == 0x66 || v == 0x67 // crystal pegs (orange/blue):
}

func (a *Area) canHookThru(v uint8) bool {
	return v == 0x00 || // no collision
		v == 0x08 || v == 0x09 || // water
		(v >= 0x0D && v <= 0x0F) || // spikes / floor ice
		v == 0x1C || v == 0x0C || // layer pass through
		v == 0x20 || // pit
		v == 0x22 || // manual stairs
		v == 0x23 || v == 0x24 || // floor switches
		(v >= 0x28 && v <= 0x2B) || // ledge tiles
		v == 0x3A || v == 0x3B || // star tiles
		v == 0x40 || // thick grass
		v == 0x4B || // warp
		v == 0x60 || // rupee tile
		(v >= 0x68 && v <= 0x6B) || // conveyors
		v == 0xB6 || // somaria start
		v == 0xBC // somaria start
}

// isHookable determines if the tile can be attached to with a hookshot
func (a *Area) isHookable(v uint8) bool {
	return v == 0x27 || // general hookable object
		v == 0x02 ||
		(v >= 0x58 && v <= 0x5D) || // chests (TODO: check $0500 table for kind)
		v&0xF0 == 0x70 // pot/peg/block
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

func (a *Area) DrawOverlays() {
	greenTint := image.NewUniform(color.NRGBA{255, 255, 0, 64})
	redTint := image.NewUniform(color.NRGBA{255, 0, 0, 96})
	// cyanTint := image.NewUniform(color.NRGBA{0, 255, 255, 64})
	// blueTint := image.NewUniform(color.NRGBA{0, 0, 255, 48})

	for c := 0; c < 0x4000; c++ {
		v := a.Reachable[c]
		if v == 0x01 {
			continue
		}

		tt := OWCoord(c)
		tr, tc := a.RowCol(tt)
		overlay := greenTint
		if v == 0x20 || v == 0x62 {
			overlay = redTint
		}

		x := int(tc) << 3
		y := int(tr) << 3
		draw.Draw(
			a.RenderedNRGBA,
			image.Rect(x, y, x+8, y+8),
			overlay,
			image.Point{},
			draw.Over,
		)
	}
}
