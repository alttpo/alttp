package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
)

func renderAll(fname string, rooms []*RoomState, rowStart int, rowCount int) {
	var err error

	const divider = 1
	supertilepx := 512 / divider

	wga := &sync.WaitGroup{}

	all := image.NewNRGBA(image.Rect(0, 0, 0x10*supertilepx, (rowCount*0x10*supertilepx)/0x10))
	// clear the image and remove alpha layer
	draw.Draw(
		all,
		all.Bounds(),
		image.NewUniform(color.NRGBA{0, 0, 0, 255}),
		image.Point{},
		draw.Src)

	greenTint := image.NewUniform(color.NRGBA{255, 255, 0, 64})
	redTint := image.NewUniform(color.NRGBA{255, 0, 0, 96})
	cyanTint := image.NewUniform(color.NRGBA{0, 255, 255, 64})
	blueTint := image.NewUniform(color.NRGBA{0, 0, 255, 48})

	black := image.NewUniform(color.RGBA{0, 0, 0, 255})
	yellow := image.NewUniform(color.RGBA{255, 255, 0, 255})
	white := image.NewUniform(color.RGBA{255, 255, 255, 255})

	for _, room := range rooms {
		st := int(room.Supertile)

		row := st/0x10 - rowStart
		col := st % 0x10
		if row < 0 || row >= rowCount {
			continue
		}

		wga.Add(1)
		go func(room *RoomState) {
			defer wga.Done()

			fmt.Printf("entrance $%02x supertile %s render start\n", room.Entrance.EntranceID, room.Supertile)

			stx := col * supertilepx
			sty := row * supertilepx

			if room.Rendered != nil {
				draw.Draw(
					all,
					image.Rect(stx, sty, stx+supertilepx, sty+supertilepx),
					room.Rendered,
					image.Point{},
					draw.Src,
				)
			}

			// highlight tiles that are reachable:
			if drawOverlays {
				maxRange := 0x2000
				if room.IsDarkRoom() {
					maxRange = 0x1000
				}

				// draw supertile over pits, bombable floors, and warps:
				for j := range room.ExitPoints {
					ep := &room.ExitPoints[j]
					if !ep.WorthMarking {
						continue
					}

					_, er, ec := ep.Point.RowCol()
					x := int(ec) << 3
					y := int(er) << 3
					fd0 := font.Drawer{
						Dst:  all,
						Src:  black,
						Face: inconsolata.Regular8x16,
						Dot:  fixed.Point26_6{fixed.I(stx + x + 1), fixed.I(sty + y + 1)},
					}
					fd1 := font.Drawer{
						Dst:  all,
						Src:  yellow,
						Face: inconsolata.Regular8x16,
						Dot:  fixed.Point26_6{fixed.I(stx + x), fixed.I(sty + y)},
					}
					stStr := fmt.Sprintf("%02X", uint16(ep.Supertile))
					fd0.DrawString(stStr)
					fd1.DrawString(stStr)
				}

				// draw supertile over stairs:
				for j := range room.Stairs {
					sn := room.StairExitTo[j]
					_, er, ec := room.Stairs[j].RowCol()

					x := int(ec) << 3
					y := int(er) << 3
					fd0 := font.Drawer{
						Dst:  all,
						Src:  black,
						Face: inconsolata.Regular8x16,
						Dot:  fixed.Point26_6{fixed.I(stx + 8 + x + 1), fixed.I(sty - 8 + y + 1 + 12)},
					}
					fd1 := font.Drawer{
						Dst:  all,
						Src:  yellow,
						Face: inconsolata.Regular8x16,
						Dot:  fixed.Point26_6{fixed.I(stx + 8 + x), fixed.I(sty - 8 + y + 12)},
					}
					stStr := fmt.Sprintf("%02X", uint16(sn))
					fd0.DrawString(stStr)
					fd1.DrawString(stStr)
				}

				for t := 0; t < maxRange; t++ {
					v := room.Reachable[t]
					if v == 0x01 {
						continue
					}

					tt := MapCoord(t)
					lyr, tr, tc := tt.RowCol()
					overlay := greenTint
					if lyr != 0 {
						overlay = cyanTint
					}
					if v == 0x20 || v == 0x62 {
						overlay = redTint
					}

					x := int(tc) << 3
					y := int(tr) << 3
					draw.Draw(
						all,
						image.Rect(stx+x, sty+y, stx+x+8, sty+y+8),
						overlay,
						image.Point{},
						draw.Over,
					)
				}

				for t, d := range room.Hookshot {
					_, tr, tc := t.RowCol()
					x := int(tc) << 3
					y := int(tr) << 3

					overlay := blueTint
					_ = d

					draw.Draw(
						all,
						image.Rect(stx+x, sty+y, stx+x+8, sty+y+8),
						overlay,
						image.Point{},
						draw.Over,
					)
				}
			}

			fmt.Printf("entrance $%02x supertile %s render complete\n", room.Entrance.EntranceID, room.Supertile)
		}(room)
	}
	wga.Wait()

	if drawNumbers {
		for r := 0; r < 0x128; r++ {
			wga.Add(1)
			go func(st int) {
				defer wga.Done()

				row := st/0x10 - rowStart
				col := st % 0x10
				if row < 0 || row >= rowCount {
					return
				}

				stx := col * supertilepx
				sty := row * supertilepx

				// draw supertile number in top-left:
				var stStr string
				if st < 0x100 {
					stStr = fmt.Sprintf("%02X", st)
				} else {
					stStr = fmt.Sprintf("%03X", st)
				}
				(&font.Drawer{
					Dst:  all,
					Src:  black,
					Face: inconsolata.Bold8x16,
					Dot:  fixed.Point26_6{fixed.I(stx + 5), fixed.I(sty + 5 + 12)},
				}).DrawString(stStr)
				(&font.Drawer{
					Dst:  all,
					Src:  white,
					Face: inconsolata.Bold8x16,
					Dot:  fixed.Point26_6{fixed.I(stx + 4), fixed.I(sty + 4 + 12)},
				}).DrawString(stStr)
			}(r)
		}
		wga.Wait()
	}

	if err = exportPNG(fmt.Sprintf("%s.png", fname), all); err != nil {
		panic(err)
	}
}

func (room *RoomState) CaptureRoomDrawFrame() {
	var tileMap [0x4000]byte
	copy(tileMap[:], room.WRAM[0x2000:0x6000])
	room.AnimatedTileMap = append(room.AnimatedTileMap, tileMap)
	room.AnimatedLayers = append(room.AnimatedLayers, room.AnimatedLayer)
}

func (room *RoomState) RenderAnimatedRoomDraw(frameDelay int) {
	wram := (&room.WRAM)[:]

	// assume WRAM has rendering state as well:
	isDark := room.IsDarkRoom()
	doBG2 := !isDark

	// INIDISP contains PPU brightness
	brightness := read8(wram, 0x13) & 0xF
	_ = brightness

	//subdes := read8(wram, 0x1D)
	n0414 := read8(wram, 0x0414)
	addColor := n0414 == 0x07
	halfColor := n0414 == 0x04
	//flip := n0414 == 0x03

	//ioutil.WriteFile(fmt.Sprintf("data/%03X.vram", st), vram, 0644)

	cgram := (*(*[0x100]uint16)(unsafe.Pointer(&wram[0xC300])))[:]

	tileset := (&room.VRAMTileSet)[:]
	var lastFrame *image.Paletted = nil

	for i, tileMap := range room.AnimatedTileMap {
		bg1wram := (*(*[0x1000]uint16)(unsafe.Pointer(&tileMap[0])))[:]
		bg2wram := (*(*[0x1000]uint16)(unsafe.Pointer(&tileMap[0x2000])))[:]

		pal := cgramToPalette(cgram)

		bg1p := [2]*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		}
		bg2p := [2]*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		}

		// render all separate BG1 and BG2 priority layers:
		layer := room.AnimatedLayers[i]
		if layer != 2 {
			renderBGsep(bg1p, bg1wram, tileset, drawBG1p0, drawBG1p1)
		}
		if doBG2 {
			renderBGsep(bg2p, bg2wram, tileset, drawBG2p0, drawBG2p1)
		}

		// swap layers depending on color math:
		if !addColor && !halfColor {
			bg1p, bg2p = bg2p, bg1p
		}

		// switch everything but the first layer to have 0 as transparent:
		//palTransp := make(color.Palette, len(pal))
		//copy(palTransp, pal)
		//palTransp[0] = color.Transparent
		//bg1p[1].Palette = palTransp
		//bg2p[0].Palette = palTransp
		//bg2p[1].Palette = palTransp

		frame := image.NewPaletted(image.Rect(0, 0, 512, 512), pal)
		ComposeToPaletted(frame, pal, bg1p, bg2p, addColor, halfColor)

		delta := frame
		dirty := false
		disposal := byte(0)
		if lastFrame != nil && optimizeGIFs {
			delta, dirty = generateDeltaFrame(lastFrame, frame)
			disposal = gif.DisposalNone
		}

		// TODO
		_ = dirty
		room.Animated.Image = append(room.Animated.Image, delta)
		room.Animated.Delay = append(room.Animated.Delay, frameDelay)
		room.Animated.Disposal = append(room.Animated.Disposal, disposal)

		lastFrame = frame
	}
}

func generateDeltaFrame(prev, curr *image.Paletted) (delta *image.Paletted, dirty bool) {
	// make a special delta palette with 255 (never used) as transparent:
	pal := make(color.Palette, len(curr.Palette))
	copy(pal, curr.Palette)

	transparentIndex := uint8(255)
	pal[transparentIndex] = color.Transparent

	delta = image.NewPaletted(image.Rect(0, 0, 512, 512), pal)
	dirty = false
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			cp := prev.ColorIndexAt(x, y)
			cc := curr.ColorIndexAt(x, y)

			if cp == cc {
				// set as transparent since nothing changed:
				delta.SetColorIndex(x, y, transparentIndex)
				continue
			}

			// use the current frame's color if it differs:
			delta.SetColorIndex(x, y, cc)
			dirty = true
		}
	}

	return
}

func renderSupertile(room *RoomState) {
	room.DrawSupertile()
}

func (room *RoomState) DrawSupertile() {
	// gfx output is:
	//  s.VRAM: $4000[0x2000] = 4bpp tile graphics
	//  s.WRAM: $2000[0x2000] = BG1 64x64 tile map  [64][64]uint16
	//  s.WRAM: $4000[0x2000] = BG2 64x64 tile map  [64][64]uint16
	//  s.WRAM:$12000[0x1000] = BG1 64x64 tile type [64][64]uint8
	//  s.WRAM:$12000[0x1000] = BG2 64x64 tile type [64][64]uint8
	//  s.WRAM: $C300[0x0200] = CGRAM palette

	wram := (&room.WRAM)[:]

	// assume WRAM has rendering state as well:
	//isDark := room.IsDarkRoom()

	// INIDISP contains PPU brightness
	brightness := read8(wram, 0x13) & 0xF
	_ = brightness

	//ioutil.WriteFile(fmt.Sprintf("data/%03X.vram", st), vram, 0644)

	// render BG layers:
	pal, bg1p, bg2p, addColor, halfColor := room.RenderBGLayers()

	//subdes := read8(wram, 0x1D)
	//n0414 := read8(wram, 0x0414)
	//flip := n0414 == 0x03

	if room.Rendered != nil {
		// subsequent GIF frames:
		frame := image.NewPaletted(image.Rect(0, 0, 512, 512), pal)
		ComposeToPaletted(frame, pal, bg1p, bg2p, addColor, halfColor)

		room.GIF.Image = append(room.GIF.Image, frame)
		room.GIF.Delay = append(room.GIF.Delay, 50)
		room.GIF.Disposal = append(room.GIF.Disposal, gif.DisposalNone)

		return
	}

	// switch everything but the first layer to have 0 as transparent:
	//order[0].Palette = pal
	//palTransp := make(color.Palette, len(pal))
	//copy(palTransp, pal)
	//palTransp[0] = color.Transparent
	//for p := 1; p < 4; p++ {
	//	order[p].Palette = palTransp
	//}

	if supertileGifs || animateRoomDrawing {
		// first GIF frames build up the layers:
		frames := [4]*image.Paletted{
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
			image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		}

		ComposeToPaletted(frames[0], pal, bg1p, bg2p, addColor, halfColor)
		ComposeToPaletted(frames[1], pal, bg1p, bg2p, addColor, halfColor)
		ComposeToPaletted(frames[2], pal, bg1p, bg2p, addColor, halfColor)
		ComposeToPaletted(frames[3], pal, bg1p, bg2p, addColor, halfColor)

		//renderBGComposedPaletted(pal, [4]*image.Paletted{order[0], blankFrame, blankFrame, blankFrame}, addColor, halfColor),
		//renderBGComposedPaletted(pal, [4]*image.Paletted{order[0], order[1], blankFrame, blankFrame}, addColor, halfColor),
		//renderBGComposedPaletted(pal, [4]*image.Paletted{order[0], order[1], order[2], blankFrame}, addColor, halfColor),
		//renderBGComposedPaletted(pal, [4]*image.Paletted{order[0], order[1], order[2], order[3]}, addColor, halfColor),

		room.GIF.Image = append(room.GIF.Image, frames[:]...)
		room.GIF.Delay = append(room.GIF.Delay, 50, 50, 50, 50)
		room.GIF.Disposal = append(room.GIF.Disposal, 0, 0, 0, 0)
	}

	g := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	ComposeToNonPaletted(g, pal, bg1p, bg2p, addColor, halfColor)

	//if isDark {
	//	// darken the room
	//	draw.Draw(
	//		g,
	//		g.Bounds(),
	//		image.NewUniform(color.RGBA64{0, 0, 0, 0x8000}),
	//		image.Point{},
	//		draw.Over,
	//	)
	//}

	//if brightness < 15 {
	//	draw.Draw(
	//		g,
	//		g.Bounds(),
	//		image.NewUniform(color.RGBA64{0, 0, 0, uint16(brightness) << 12}),
	//		image.Point{},
	//		draw.Over,
	//	)
	//}

	if drawSpriteHitboxes {
		// reset WRAM to initial supertile load:
		room.WRAM = room.WRAMAfterLoaded

		if oopsAll >= 0 {
			// replace all enemy sprites with this sprite ID:
		sprLoop:
			for i := 0; i < 16; i++ {
				// dead?
				if room.WRAM[0x0DD0+i] == 0 {
					continue
				}

				et := &room.WRAM[0x0E20+i]

				// exclude specific sprite IDs:
				for _, id := range excludeSprites {
					// ignore switches
					// -excludesprites=04,05,06,07,1E,21
					if *et == id {
						continue sprLoop
					}
				}

				// replace sprite ID:
				*et = uint8(oopsAll)
			}
		}

		// run a few frames:
		for i := 0; i < 16; i++ {
			if err := room.e.ExecAtUntil(b00RunSingleFramePC, 0, 0x400000); err != nil {
				fmt.Fprintln(os.Stderr, err)
				break
			}

			if room.e.Bus.Read8(0x7E0010) == 0x07 && room.e.Bus.Read8(0x7E0011) == 0x00 {
				break
			}
		}

		room.DrawSpriteHitboxes(g)

		room.WRAM = room.WRAMAfterLoaded
	}

	// store full underworld rendering for inclusion into EG map:
	room.Rendered = g

	if drawRoomPNGs {
		if err := exportPNG(fmt.Sprintf("%03X.png", uint16(room.Supertile)), g); err != nil {
			panic(err)
		}
	}

	if drawBGLayerPNGs {
		if err := exportPNG(fmt.Sprintf("%03X.bg1.0.png", uint16(room.Supertile)), bg1p[0]); err != nil {
			panic(err)
		}
		if err := exportPNG(fmt.Sprintf("%03X.bg1.1.png", uint16(room.Supertile)), bg1p[1]); err != nil {
			panic(err)
		}
		if err := exportPNG(fmt.Sprintf("%03X.bg2.0.png", uint16(room.Supertile)), bg2p[0]); err != nil {
			panic(err)
		}
		if err := exportPNG(fmt.Sprintf("%03X.bg2.1.png", uint16(room.Supertile)), bg2p[1]); err != nil {
			panic(err)
		}
	}
}

func (room *RoomState) RenderBGLayers() (
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	addColor, halfColor bool,
) {
	return renderBGLayers(&room.WRAM, (&room.VRAMTileSet)[:])
}

func renderBGLayers(wramArray *WRAMArray, tileset []uint8) (
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	addColor, halfColor bool,
) {
	wram := wramArray[:]

	// assume WRAM has rendering state as well:
	//isDark := room.IsDarkRoom()
	isDark := read8(wram, 0xC005) != 0

	// INIDISP contains PPU brightness
	//brightness := read8(wram, 0x13) & 0xF
	//_ = brightness

	//ioutil.WriteFile(fmt.Sprintf("%03X.vram", st), vram, 0644)

	// extract palette:
	cgram := (*(*[0x100]uint16)(unsafe.Pointer(&wram[0xC300])))[:]
	pal = cgramToPalette(cgram)

	// render BG layers:
	bg1p = [2]*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
	}
	bg2p = [2]*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
	}

	// render all separate BG1 and BG2 priority layers:
	{
		bg2wram := (*(*[0x1000]uint16)(unsafe.Pointer(&wram[0x2000])))[:]
		renderBGsep(bg2p, bg2wram, tileset, drawBG1p0, drawBG1p1)
		// if !isDark {
		bg1wram := (*(*[0x1000]uint16)(unsafe.Pointer(&wram[0x4000])))[:]
		renderBGsep(bg1p, bg1wram, tileset, drawBG2p0, drawBG2p1)
		// }
	}

	//subdes := read8(wram, 0x1D)
	n0414 := read8(wram, 0x0414)
	addColor = n0414 == 0x07
	halfColor = n0414 == 0x04
	flip := n0414 == 0x03

	// swap bg1 and bg2 if color math is involved:
	// if !addColor && !halfColor && !flip {
	// 	bg1p, bg2p = bg2p, bg1p
	// }
	_ = flip
	_ = isDark

	return
}

func renderOWBGLayers(wramArray *WRAMArray, bg1tilemap []uint16, bg2tilemap []uint16, tileset []byte) (
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	addColor, halfColor bool,
) {
	wram := wramArray[:]

	// assume WRAM has rendering state as well:
	//isDark := room.IsDarkRoom()
	//isDark := read8(wram, 0xC005) != 0

	// INIDISP contains PPU brightness
	//brightness := read8(wram, 0x13) & 0xF
	//_ = brightness

	//ioutil.WriteFile(fmt.Sprintf("%03X.vram", st), vram, 0644)

	// extract palette:
	cgram := (*(*[0x100]uint16)(unsafe.Pointer(&wram[0xC300])))[:]
	pal = cgramToPalette(cgram)

	// render BG layers:
	bg1p = [2]*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
	}
	bg2p = [2]*image.Paletted{
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
		image.NewPaletted(image.Rect(0, 0, 512, 512), pal),
	}

	// render all separate BG1 and BG2 priority layers:
	renderVRAMBG(bg1p, bg1tilemap, tileset, drawBG1p0, drawBG1p1)
	renderVRAMBG(bg2p, bg2tilemap, tileset, drawBG2p0, drawBG2p1)

	//subdes := read8(wram, 0x1D)
	n0414 := read8(wram, 0x0414)
	addColor = n0414 == 0x07
	halfColor = n0414 == 0x04
	flip := n0414 == 0x03

	// swap bg1 and bg2 if color math is involved:
	if !addColor && !halfColor && !flip {
		bg1p, bg2p = bg2p, bg1p
	}

	return
}

func drawShadowedString(g draw.Image, clr image.Image, dot fixed.Point26_6, s string) {
	// shadow:
	for oy := -1; oy <= 1; oy++ {
		for ox := -1; ox <= 1; ox++ {
			(&font.Drawer{
				Dst:  g,
				Src:  image.Black,
				Face: inconsolata.Bold8x16,
				Dot:  fixed.Point26_6{X: dot.X + fixed.I(ox), Y: dot.Y + fixed.I(oy)},
			}).DrawString(s)
		}
	}

	// regular label:
	(&font.Drawer{
		Dst:  g,
		Src:  clr,
		Face: inconsolata.Bold8x16,
		Dot:  dot,
	}).DrawString(s)
}

func drawOutlineBox(g draw.Image, clr image.Image, x, y int, w, h int) {
	// outline box:
	draw.Draw(g, image.Rect(x, y, x+w, y+1), clr, image.Point{}, draw.Over)
	draw.Draw(g, image.Rect(x+w, y, x+w+1, y+h), clr, image.Point{}, draw.Over)
	draw.Draw(g, image.Rect(x, y+h, x+w, y+h+1), clr, image.Point{}, draw.Over)
	draw.Draw(g, image.Rect(x, y, x+1, y+h), clr, image.Point{}, draw.Over)
}

func drawCircle(img draw.Image, x0, y0, r int, c color.Color) {
	x, y, dx, dy := r-1, 0, 1, 1
	err := dx - (r * 2)

	for x > y {
		img.Set(x0+x, y0+y, c)
		img.Set(x0+y, y0+x, c)
		img.Set(x0-y, y0+x, c)
		img.Set(x0-x, y0+y, c)
		img.Set(x0-x, y0-y, c)
		img.Set(x0-y, y0-x, c)
		img.Set(x0+y, y0-x, c)
		img.Set(x0+x, y0-y, c)

		if err <= 0 {
			y++
			err += dy
			dy += 2
		}
		if err > 0 {
			x--
			dx += 2
			err += dx - (r * 2)
		}
	}
}

type filledCircleMask struct {
	p image.Point
	r int
	a color.Alpha
}

func (c *filledCircleMask) ColorModel() color.Model {
	return color.AlphaModel
}

func (c *filledCircleMask) Bounds() image.Rectangle {
	return image.Rect(c.p.X-c.r, c.p.Y-c.r, c.p.X+c.r, c.p.Y+c.r)
}

func (c *filledCircleMask) At(x, y int) color.Color {
	xx, yy, rr := float64(x-c.p.X)+0.5, float64(y-c.p.Y)+0.5, float64(c.r)
	if xx*xx+yy*yy < rr*rr {
		return c.a
	}
	return color.Alpha{0}
}

func (room *RoomState) DrawSpriteHitboxes(g draw.Image) {
	wram := (&room.WRAM)[:]

	//black := image.NewUniform(color.RGBA{0, 0, 0, 255})
	yellow := image.NewUniform(color.RGBA{255, 255, 0, 255})
	//red := image.NewUniform(color.RGBA{255, 48, 48, 255})
	alpha := color.Alpha{60}
	alphaU := image.NewUniform(alpha)

	roomX, roomY := room.Supertile.AbsTopLeft()

	getXY := func(i uint32, wram []byte) (x, y uint16) {
		yl, yh := read8(wram, 0x0D00+i), read8(wram, 0x0D20+i)
		xl, xh := read8(wram, 0x0D10+i), read8(wram, 0x0D30+i)
		y = uint16(yl) | uint16(yh)<<8
		x = uint16(xl) | uint16(xh)<<8
		return
	}
	getHitBox := func(i uint32, wram []byte) (x, y, w, h int) {
		// hitbox:
		hb := uint32(read8(wram, 0x0F60+i) & 0x1F)

		// find hitbox coords:
		x = int(int8(room.e.Bus.Read8(0x06F735 + hb)))
		w = int(int8(room.e.Bus.Read8(0x06F775 + hb)))
		y = int(int8(room.e.Bus.Read8(0x06F795 + hb)))
		h = int(int8(room.e.Bus.Read8(0x06F7D5 + hb)))

		return
	}

	// draw sprites:
	for i := uint32(0); i < 16; i++ {
		i := i
		// AI state:
		st := read8(wram, 0x0DD0+i)
		if st == 0 {
			continue
		}

		clr := yellow

		// determine if in bounds:
		x, y := getXY(i, wram)
		if !room.IsAbsInBounds(x, y) {
			continue
		}

		//enemy type:
		et := read8(wram, 0x0E20+i)

		//(&font.Drawer{
		//	Dst:  g,
		//	Src:  clr,
		//	Face: inconsolata.Bold8x16,
		//	Dot:  fixed.Point26_6{X: fixed.I(lx), Y: fixed.I(ly + 12)},
		//}).DrawString(fmt.Sprintf("%02x", et))

		// find quadrant top-left x,y:
		//bgX := roomX + ((x - roomX) & ^uint16(0x7F))
		//bgY := roomY + ((y - roomY) & ^uint16(0x7F))

		bgX := int(x) - 0x80
		if bgX < 0 {
			bgX = 0
		}
		bgY := int(y) - 0x80
		if bgY < 0 {
			bgY = 0
		}

		c := image.NewRGBA(g.Bounds())
		a := image.NewAlpha(g.Bounds())

		drawOAMSprites := func(e *System) {
			if false {
				// dump VRAM to a PNG file for debugging:
				dbg := image.NewRGBA(image.Rect(0, 0, 32*8, 16*8))
				for tp := uint32(0); tp < uint32(0x4000); tp += 0x20 {
					draw4bppTile(
						e.VRAM,
						e.HWIO.PPU.ObjTilemapAddress+tp,
						8,
						8,
						false,
						false,
						int(((tp&0x01FF)>>2)+(tp&0xE000)>>6),
						int((tp&0x1E00)>>6),
						4,
						func(x, y int, idx uint8) {
							dbg.Set(x, y, cgramRGBA((*[0x100]uint16)(unsafe.Pointer(&e.WRAM[0xC500]))[idx]))
						},
					)
				}
				_ = exportPNG(fmt.Sprintf("vram.%03X.png", uint16(room.Supertile)), dbg)
			}

			renderOAMSprites(
				g,
				(*[0x100]uint16)(unsafe.Pointer(&e.WRAM[0xC500])),
				e.VRAM,
				e.HWIO.PPU.ObjTilemapAddress,
				e.HWIO.PPU.ObjNameSelect,
				(*[0x200]byte)(unsafe.Pointer(&e.WRAM[0x0800])),
				(*[0x80]byte)(unsafe.Pointer(&e.WRAM[0x0A20])),
				bgX-int(roomX),
				bgY-int(roomY),
			)
		}

		{
			var renderHitBoxes func(e *System) bool

			switch et {
			case 0x7E, 0x7F: // fire bar
				renderHitBoxes = func(e *System) bool {
					// render rough hitboxes from OAM:
					for j := 0; j < 0x80; j++ {
						sy := int(e.WRAM[0x800+(j<<2)+1])
						if sy >= 0xF0 {
							continue
						}

						sx := int(e.WRAM[0x800+(j<<2)+0])
						// use the extra table for X high bit
						if e.WRAM[0x0A20+j]&1 != 0 {
							sx = int(^uint8(sx)) + 1 - 512
						}

						ax := (sx + int(bgX)) - int(roomX)
						ay := (sy + int(bgY)) - int(roomY)

						rect := image.Rect(
							ax,
							ay,
							ax+0x18,
							ay+0x10,
						)
						draw.Draw(
							c,
							rect,
							clr,
							image.Point{},
							draw.Src,
						)
						draw.Draw(
							a,
							rect,
							alphaU,
							image.Point{},
							draw.Src,
						)
					}
					return true
				}
				break
			default:
				renderHitBoxes = func(e *System) bool {
					wram := e.WRAM[:]

					// AI state:
					st := read8(wram, 0x0DD0+i)
					if st == 0 {
						return false
					}

					// determine if in bounds:
					x, y := getXY(i, wram)
					if !room.IsAbsInBounds(x, y) {
						return true
					}

					// hitbox:
					hbX, hbY, hbW, hbH := getHitBox(i, wram)

					// screen coords:
					lx := (int(x) & 0x1FF) + hbX
					ly := (int(y) & 0x1FF) + hbY

					// fill:
					draw.Draw(c, image.Rect(lx, ly, lx+hbW+1, ly+hbH+1), clr, image.Point{}, draw.Src)
					draw.Draw(a, image.Rect(lx, ly, lx+hbW+1, ly+hbH+1), alphaU, image.Point{}, draw.Src)

					return true
				}
				break
			}

			e := System{}
			_ = e.InitEmulatorFrom(room.e)
			//e.LoggerCPU = os.Stdout

			// set data bank:
			e.CPU.RDBR = 0x06

			// clear i-frames and invuln:
			e.WRAM[0x031F] = 0
			e.WRAM[0x037B] = 0

			for j := 0; j < 0x80; j++ {
				e.WRAM[0x0800+(j<<2)+0] = 0
				e.WRAM[0x0800+(j<<2)+1] = 0xF0
				e.WRAM[0x0800+(j<<2)+2] = 0
				e.WRAM[0x0800+(j<<2)+3] = 0
				e.WRAM[0x0A20+j] = 0
			}

			// ensure sprite on screen:
			e.Bus.Write16(0x7E00E2, uint16(bgX))
			e.Bus.Write16(0x7E00E8, uint16(bgY))
			// set link x/y coords for collision detection:
			e.Bus.Write16(0x7E0022, uint16(int(x))) // X
			e.Bus.Write16(0x7E0020, uint16(int(y))) // Y

			// execute Sprite_Main up until `LDX #$0F` which begins sprite_executesingle loop:
			if err := e.ExecAtUntil(0x068328, 0x06839F, 0x200000); err != nil {
				fmt.Fprintln(os.Stderr, err)
				break
			}

			frameCount := uint16(1)
			if isPeriodicEnemy(et) {
				frameCount = 0x200
			}

			// run through lots of frames and execute sprite logic for the given sprite:
			for j := uint16(0); j < frameCount; j++ {
				// set X register to sprite slot:
				e.Bus.Write16(0x7E0FA0, uint16(i))
				e.CPU.RX = uint16(i)
				e.CPU.RXl = uint8(i)
				// 8 bit mode:
				e.CPU.M = 1
				e.CPU.X = 1

				// 0x0684DD // JSR Sprite_ExecuteSingle with X=sprite slot
				spriteExecuteSingle := 0x0683A4 | fastRomBank

				// execute logic for the sprite:
				if err := e.ExecAtUntil(spriteExecuteSingle, spriteExecuteSingle+3, 0x200000); err != nil {
					fmt.Fprintln(os.Stderr, err)
					break
				}

				if j == 0 {
					drawOAMSprites(&e)
				}

				if !renderHitBoxes(&e) {
					break
				}
			}
		}

		// draw the color masked with alpha onto the src:
		draw.DrawMask(
			g,
			g.Bounds(),
			c,
			image.Point{},
			a,
			image.Point{},
			draw.Over,
		)
	}
}

func isPeriodicEnemy(et uint8) bool {
	if et == 0x7E || et == 0x7F { // fire bar
		return true
	}
	if et >= 0x5D && et <= 0x60 { // rollers
		return true
	}
	return false
}

func renderOAMSpriteNumbers(g draw.Image, vram *VRAMArray, oam *[0x200]byte, oamx *[0x80]byte, qx int, qy int) {
	for i := 0; i < 128; i++ {
		y := int(oam[i<<2+1])
		if y >= 0xF0 {
			continue
		}

		bits := oamx[i] & 3

		var x int
		if bits&1 != 0 {
			x = int(^oam[i<<2+0]) + 1 - 512
		} else {
			x = int(oam[i<<2+0])
		}

		t := int(oam[i<<2+2])
		tn := int(oam[i<<2+3]) & 1
		fv := oam[i<<2+3]&0x80 != 0
		fh := oam[i<<2+3]&0x40 != 0
		pri := oam[i<<2+3] & 0x30 >> 4
		pal := oam[i<<2+3] & 0xE >> 1

		//ta := room.e.HWIO.PPU.ObjTilemapAddress + uint32(t)*0x20
		//ta += uint32(tn) * room.e.HWIO.PPU.ObjNameSelect
		//ta &= 0xFFFF
		//room.e.VRAM[ta]
		drawShadowedString(
			g,
			image.White,
			fixed.Point26_6{X: fixed.I(qx + x), Y: fixed.I(qy + y + 12)},
			fmt.Sprintf("%03X", t|tn<<8),
		)
		_, _, _, _ = fv, fh, pri, pal
	}
}

func draw4bppTile(
	vram *VRAMArray,
	tp uint32,
	w uint32,
	h uint32,
	fh bool,
	fv bool,
	x int,
	y int,
	pal byte,
	setPx func(x, y int, idx uint8),
) {
	// 4bpp:
	for ty := uint32(0); ty < h; ty++ {
		var fty uint32
		if fv {
			// vertical flip:
			fty = h - 1 - ty
		} else {
			fty = ty
		}

		for tx := uint32(0); tx < w; tx++ {
			var ftx uint32
			if fh {
				// horizontal flip:
				ftx = w - 1 - tx
			} else {
				// normal:
				ftx = tx
			}

			// calculate tilemap address:
			ta := tp + ((ftx >> 3) << 5) + ((fty >> 3) << 9) + ((fty & 7) << 1)
			// read 4 bytes:
			d0, d1, d2, d3 := vram[ta+0], vram[ta+1], vram[ta+16], vram[ta+17]

			// calculate bit mask of X:
			m := uint8(1) << (7 - (ftx & 7))

			// compute 4-bit color index:
			ci := uint8(0)
			if d0&m != 0 {
				ci |= 0b0001
			}
			if d1&m != 0 {
				ci |= 0b0010
			}
			if d2&m != 0 {
				ci |= 0b0100
			}
			if d3&m != 0 {
				ci |= 0b1000
			}

			if ci == 0 {
				continue
			}

			setPx(x+int(tx), y+int(ty), 128+(pal<<4)+ci)
		}
	}
}

func renderOAMSpritesPrioritizedPaletted(
	obj [4]*image.Paletted,
	vram *VRAMArray,
	objTileMapAddress uint32,
	objNameSelect uint32,
	oam *[0x200]byte,
	oamx *[0x80]byte,
	qx int,
	qy int,
) {
	for i := 127; i >= 0; i-- {
		y := int(oam[i<<2+1])
		if y == 0xF0 {
			continue
		}
		if y > 0xF0 {
			y -= 256
		}

		bits := oamx[i] & 3

		var x int
		if bits&1 != 0 {
			x = int(^oam[i<<2+0]) + 1 - 512
		} else {
			x = int(oam[i<<2+0])
		}

		// simplification of sprite sizing:
		var w uint32 = 8
		var h uint32 = 8
		if bits&2 != 0 {
			w = 16
			h = 16
		}

		t := int(oam[i<<2+2])
		tn := int(oam[i<<2+3]) & 1
		fv := oam[i<<2+3]&0x80 != 0
		fh := oam[i<<2+3]&0x40 != 0
		pri := (oam[i<<2+3] & 0x30) >> 4
		pal := (oam[i<<2+3] & 0xE) >> 1

		tp := objTileMapAddress + uint32(t)*0x20
		tp += objNameSelect * uint32(tn)
		tp &= 0xFFFF

		draw4bppTile(vram, tp, w, h, fh, fv, qx+x, qy+y+1, pal, obj[pri].SetColorIndex)
	}
}

func renderOAMSprites(
	g draw.Image,
	cgram *[0x100]uint16,
	vram *VRAMArray,
	objTileMapAddress uint32,
	objNameSelect uint32,
	oam *[0x200]byte,
	oamx *[0x80]byte,
	qx int,
	qy int,
) {
	setPx := func(x, y int, idx uint8) {
		g.Set(x, y, cgramRGBA(cgram[idx]))
	}

	for i := 127; i >= 0; i-- {
		y := int(oam[i<<2+1])
		if y == 0xF0 {
			continue
		}
		if y > 0xF0 {
			y -= 256
		}

		bits := oamx[i] & 3

		var x int
		if bits&1 != 0 {
			x = int(^oam[i<<2+0]) + 1 - 512
		} else {
			x = int(oam[i<<2+0])
		}

		// simplification of sprite sizing:
		var w uint32 = 8
		var h uint32 = 8
		if bits&2 != 0 {
			w = 16
			h = 16
		}

		t := int(oam[i<<2+2])
		tn := int(oam[i<<2+3]) & 1
		fv := oam[i<<2+3]&0x80 != 0
		fh := oam[i<<2+3]&0x40 != 0
		pri := oam[i<<2+3] & 0x30 >> 4
		pal := oam[i<<2+3] & 0xE >> 1

		tp := objTileMapAddress + uint32(t)*0x20
		tp += objNameSelect * uint32(tn)
		tp &= 0xFFFF

		draw4bppTile(vram, tp, w, h, fh, fv, qx+x, qy+y+1, pal, setPx)

		//drawShadowedString(
		//	g,
		//	image.White,
		//	fixed.Point26_6{X: fixed.I(qx + x), Y: fixed.I(qy + y + 12)},
		//	fmt.Sprintf("%03X", t|tn<<8),
		//)

		_ = pri
	}
}

func renderOAMSpritesPrioritizedPalettedFromWRAM(obj [4]*image.Paletted, e *System, bgX, bgY, roomX, roomY int) {
	renderOAMSpritesPrioritizedPaletted(
		obj,
		e.VRAM,
		e.HWIO.PPU.ObjTilemapAddress,
		e.HWIO.PPU.ObjNameSelect,
		(*[0x200]byte)(unsafe.Pointer(&e.WRAM[0x0800])),
		(*[0x80]byte)(unsafe.Pointer(&e.WRAM[0x0A20])),
		bgX-roomX,
		bgY-roomY,
	)
}

func renderOAMSpritesFromWRAM(g draw.Image, e *System, bgX, bgY, roomX, roomY int) {
	renderOAMSprites(
		g,
		(*[0x100]uint16)(unsafe.Pointer(&e.WRAM[0xC500])),
		e.VRAM,
		e.HWIO.PPU.ObjTilemapAddress,
		e.HWIO.PPU.ObjNameSelect,
		(*[0x200]byte)(unsafe.Pointer(&e.WRAM[0x0800])),
		(*[0x80]byte)(unsafe.Pointer(&e.WRAM[0x0A20])),
		bgX-roomX,
		bgY-roomY,
	)
}

func (room *RoomState) RenderSpriteHitBoxes(g draw.Image) {
	wram := (&room.WRAM)[:]

	//black := image.NewUniform(color.RGBA{0, 0, 0, 255})
	yellow := image.NewUniform(color.RGBA{255, 255, 0, 255})
	red := image.NewUniform(color.RGBA{255, 48, 48, 255})

	// draw sprites:
	for i := uint32(0); i < 16; i++ {
		clr := yellow

		// Initial AI state on load:
		//initialAIState := read8(room.WRAMAfterLoaded[:], 0x0DD0+i)
		//if initialAIState == 0 {
		//	// nothing was ever here:
		//	continue
		//}

		// determine if in bounds:
		yl, yh := read8(wram, 0x0D00+i), read8(wram, 0x0D20+i)
		xl, xh := read8(wram, 0x0D10+i), read8(wram, 0x0D30+i)
		y := uint16(yl) | uint16(yh)<<8
		x := uint16(xl) | uint16(xh)<<8
		if !room.IsAbsInBounds(x, y) {
			continue
		}

		// AI state:
		st := read8(wram, 0x0DD0+i)

		// enemy type:
		// et := read8(wram, 0x0E20+i)

		var lx, ly int
		if true {
			lx = int(x) & 0x1FF
			ly = int(y) & 0x1FF
		} else {
			coord := AbsToMapCoord(x, y, 0)
			_, row, col := coord.RowCol()
			lx = int(col << 3)
			ly = int(row << 3)
		}

		bgX := int(x) - 0x80
		if bgX < 0 {
			bgX = 0
		}
		bgY := int(y) - 0x80
		if bgY < 0 {
			bgY = 0
		}

		//fmt.Printf(
		//	"%02x @ abs(%04x, %04x) -> map(%04x, %04x)\n",
		//	et,
		//	x,
		//	y,
		//	col,
		//	row,
		//)

		hb := hitbox[read8(wram, 0x0F60+i)&0x1F]

		if st == 0 {
			// dead:
			clr = red
		}

		// draw OAM:
		drawOutlineBox(g, clr, lx+hb.X, ly+hb.Y, hb.W, hb.H)

		// colored number label:
		// drawShadowedString(g, clr, fixed.Point26_6{X: fixed.I(lx), Y: fixed.I(ly + 12)}, fmt.Sprintf("%02X", et))
	}

	// draw Link:
	{
		x := read16(wram, 0x22)
		y := read16(wram, 0x20)
		var lx, ly int
		if true {
			lx = int(x) & 0x1FF
			ly = int(y) & 0x1FF
		} else {
			coord := AbsToMapCoord(x, y, 0)
			_, row, col := coord.RowCol()
			lx = int(col << 3)
			ly = int(row << 3)
		}

		green := image.NewUniform(color.RGBA{0, 255, 0, 255})
		drawOutlineBox(g, green, lx, ly, 16, 16)
		drawShadowedString(g, green, fixed.Point26_6{X: fixed.I(lx), Y: fixed.I(ly + 12)}, "LK")
	}
}

func newBlankFrame() *image.Paletted {
	return image.NewPaletted(
		image.Rect(0, 0, 512, 512),
		color.Palette{color.Transparent},
	)
}

// saturate a 16-bit value:
func sat(v uint32) uint16 {
	if v > 0xffff {
		return 0xffff
	}
	return uint16(v)
}

// prefer p1's color unless it's zero:
func pick(c0, c1 uint8) uint8 {
	if c1 != 0 {
		return c1
	} else {
		return c0
	}
}

func ComposeToNonPaletted(
	dst draw.Image,
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	addColor bool,
	halfColor bool,
) {
	mx := dst.Bounds().Min.X
	my := dst.Bounds().Min.Y
	if halfColor {
		// color math: add half
		for y := 0; y < 512; y++ {
			for x := 0; x < 512; x++ {
				bg1c := pick(bg1p[0].ColorIndexAt(x, y), bg1p[1].ColorIndexAt(x, y))
				bg2c := pick(bg2p[0].ColorIndexAt(x, y), bg2p[1].ColorIndexAt(x, y))
				if bg2c != 0 && bg1c != 0 {
					r1, g1, b1, _ := pal[bg1c].RGBA()
					r2, g2, b2, _ := pal[bg2c].RGBA()
					c := color.RGBA64{
						R: sat(r1>>1 + r2>>1),
						G: sat(g1>>1 + g2>>1),
						B: sat(b1>>1 + b2>>1),
						A: 0xffff,
					}
					dst.Set(mx+x, my+y, c)
				} else {
					c := pick(bg1c, bg2c)
					dst.Set(mx+x, my+y, pal[c])
				}
			}
		}
	} else if addColor {
		// color math: add
		for y := 0; y < 512; y++ {
			for x := 0; x < 512; x++ {
				bg1c := pick(bg1p[0].ColorIndexAt(x, y), bg1p[1].ColorIndexAt(x, y))
				bg2c := pick(bg2p[0].ColorIndexAt(x, y), bg2p[1].ColorIndexAt(x, y))
				r1, g1, b1, _ := pal[bg1c].RGBA()
				r2, g2, b2, _ := pal[bg2c].RGBA()
				c := color.RGBA64{
					R: sat(r1 + r2),
					G: sat(g1 + g2),
					B: sat(b1 + b2),
					A: 0xffff,
				}
				dst.Set(mx+x, my+y, c)
			}
		}
	} else {
		// no color math:
		for y := 0; y < 512; y++ {
			for x := 0; x < 512; x++ {
				c0 := bg1p[0].ColorIndexAt(x, y)
				c1 := bg1p[1].ColorIndexAt(x, y)
				c2 := bg2p[0].ColorIndexAt(x, y)
				c3 := bg2p[1].ColorIndexAt(x, y)
				c := pick(pick(c0, c1), pick(c2, c3))
				dst.Set(mx+x, my+y, pal[c])
			}
		}
	}
}

func ComposeToNonPalettedOW(
	dst draw.Image,
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	wh int,
	addColor bool,
	halfColor bool,
) {
	mx := dst.Bounds().Min.X
	my := dst.Bounds().Min.Y
	if halfColor {
		// color math: add half
		for y := 0; y < wh*8; y++ {
			for x := 0; x < wh*8; x++ {
				bg1c := pick(bg1p[0].ColorIndexAt(x, y), bg1p[1].ColorIndexAt(x, y))
				bg2c := pick(bg2p[0].ColorIndexAt(x, y), bg2p[1].ColorIndexAt(x, y))
				if bg2c != 0 && bg1c != 0 {
					r1, g1, b1, _ := pal[bg1c].RGBA()
					r2, g2, b2, _ := pal[bg2c].RGBA()
					c := color.RGBA64{
						R: sat(r1>>1 + r2>>1),
						G: sat(g1>>1 + g2>>1),
						B: sat(b1>>1 + b2>>1),
						A: 0xffff,
					}
					dst.Set(mx+x, my+y, c)
				} else {
					c := pick(bg1c, bg2c)
					dst.Set(mx+x, my+y, pal[c])
				}
			}
		}
	} else if addColor {
		// color math: add
		for y := 0; y < wh*8; y++ {
			for x := 0; x < wh*8; x++ {
				bg1c := pick(bg1p[0].ColorIndexAt(x, y), bg1p[1].ColorIndexAt(x, y))
				bg2c := pick(bg2p[0].ColorIndexAt(x, y), bg2p[1].ColorIndexAt(x, y))
				r1, g1, b1, _ := pal[bg1c].RGBA()
				r2, g2, b2, _ := pal[bg2c].RGBA()
				c := color.RGBA64{
					R: sat(r1 + r2),
					G: sat(g1 + g2),
					B: sat(b1 + b2),
					A: 0xffff,
				}
				dst.Set(mx+x, my+y, c)
			}
		}
	} else {
		// no color math:
		for y := 0; y < wh*8; y++ {
			for x := 0; x < wh*8; x++ {
				c0 := bg1p[0].ColorIndexAt(x, y)
				c1 := bg1p[1].ColorIndexAt(x, y)
				c2 := bg2p[0].ColorIndexAt(x, y)
				c3 := bg2p[1].ColorIndexAt(x, y)
				c := pick(pick(c0, c1), pick(c2, c3))
				dst.Set(mx+x, my+y, pal[c])
			}
		}
	}
}

func ComposeToPaletted(
	dst *image.Paletted,
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	addColor bool,
	halfColor bool,
) {
	// store mixed colors in second half of palette which is unused by BG layers:
	hc := uint8(128)
	mixedColors := make(map[uint16]uint8, 0x200)

	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			var c uint8
			c0 := bg2p[0].ColorIndexAt(x, y)
			c1 := bg2p[1].ColorIndexAt(x, y)
			c2 := bg1p[0].ColorIndexAt(x, y)
			c3 := bg1p[1].ColorIndexAt(x, y)

			m1 := pick(c0, c1)
			m2 := pick(c2, c3)

			if addColor || halfColor {
				if m2 == 0 {
					c = m1
				} else {
					key := uint16(m1) | uint16(m2)<<8

					var ok bool
					if c, ok = mixedColors[key]; !ok {
						c = hc
						r1, g1, b1, _ := pal[m1].RGBA()
						r2, g2, b2, _ := pal[m2].RGBA()
						if halfColor {
							pal[c] = color.RGBA64{
								R: sat(r1>>1 + r2>>1),
								G: sat(g1>>1 + g2>>1),
								B: sat(b1>>1 + b2>>1),
								A: 0xffff,
							}
						} else {
							pal[c] = color.RGBA64{
								R: sat(r1 + r2),
								G: sat(g1 + g2),
								B: sat(b1 + b2),
								A: 0xffff,
							}
						}
						mixedColors[key] = c
						hc++
					}
				}
			} else {
				c = pick(m1, m2)
			}

			dst.SetColorIndex(x, y, c)
		}
	}

	dst.Palette = pal
}

type PPURegs struct {
	TM       uint8 // 0x1C  == $212C
	TS       uint8 // 0x1D  == $212D
	CGWSEL   uint8 // 0x99  == $2130
	CGADDSUB uint8 // 0x9A  == $2131
}

func (p *PPURegs) UsesColorMath() bool {
	return p.CGADDSUB&0x13 != 0
}

func ComposePrioritizedToPaletted(
	dst *image.Paletted,
	pal color.Palette,
	bg1p [2]*image.Paletted,
	bg2p [2]*image.Paletted,
	obj [4]*image.Paletted,
	ppu PPURegs,
) {
	// ordered from highest to lowest priority:
	layers := [8]*image.Paletted{
		obj[3],
		bg1p[1],
		bg2p[1],
		obj[2],
		bg1p[0],
		bg2p[0],
		obj[1],
		obj[0],
	}

	objEnable := [2]bool{
		ppu.TM&0x10 != 0,
		ppu.TS&0x10 != 0,
	}
	bg1Enable := [2]bool{
		ppu.TM&0x01 != 0,
		ppu.TS&0x01 != 0,
	}
	bg2Enable := [2]bool{
		ppu.TM&0x02 != 0,
		ppu.TS&0x02 != 0,
	}
	// dont care about BG3 or BG4

	// main/sub enablements are aligned with priority order above:
	layerEnable := [8][2]bool{
		objEnable,
		bg1Enable,
		bg2Enable,
		objEnable,
		bg1Enable,
		bg2Enable,
		objEnable,
		objEnable,
	}

	preventMath := (ppu.CGWSEL>>4)&3 > 0

	mathEnable := [9]bool{
		ppu.CGADDSUB&0x10 != 0, // obj2
		ppu.CGADDSUB&0x01 != 0, // bg1
		ppu.CGADDSUB&0x02 != 0, // bg2
		ppu.CGADDSUB&0x10 != 0, // obj2
		ppu.CGADDSUB&0x01 != 0, // bg1
		ppu.CGADDSUB&0x02 != 0, // bg2
		ppu.CGADDSUB&0x10 != 0, // obj2
		ppu.CGADDSUB&0x10 != 0, // obj2
		ppu.CGADDSUB&0x20 != 0, // col
	}

	subColorSource := ppu.CGWSEL&0x02 != 0
	halve := ppu.CGADDSUB&0x40 != 0
	subtractColors := ppu.CGADDSUB&0x80 != 0

	// store mixed colors in second half of palette which is unused by BG layers:
	mixedColors := make(map[uint16]uint8, 0x200)
	usedColors := [256]bool{}
	usedColors[0] = true
	hc := uint8(1)

	dst.Palette = pal

	if ppu.UsesColorMath() {
		// discover colors in use for color-math palettization:
		for y := 0; y < 512; y++ {
			for x := 0; x < 512; x++ {
				// main:
				for l := 0; l < 8; l++ {
					if !layerEnable[l][0] {
						continue
					}
					c := layers[l].ColorIndexAt(x, y)
					if c == 0 {
						continue
					}

					usedColors[c] = true
					if c == hc {
						hc++
					}
					break
				}
				if subColorSource {
					// sub:
					for l := 0; l < 8; l++ {
						if !layerEnable[l][1] {
							continue
						}
						c := layers[l].ColorIndexAt(x, y)
						if c == 0 {
							continue
						}

						usedColors[c] = true
						if c == hc {
							hc++
						}
						break
					}
				}
			}
		}
		// find next unused color:
		for usedColors[hc] {
			hc++
		}
	}

	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			lm := 8
			cm, cs := uint8(0), uint8(0)
			// main:
			for l := 0; l < 8; l++ {
				if !layerEnable[l][0] {
					continue
				}
				c := layers[l].ColorIndexAt(x, y)
				if c == 0 {
					continue
				}

				cm = c
				lm = l
				break
			}
			if subColorSource {
				// sub:
				for l := 0; l < 8; l++ {
					if !layerEnable[l][1] {
						continue
					}
					c := layers[l].ColorIndexAt(x, y)
					if c == 0 {
						continue
					}

					cs = c
					break
				}
			}

			var c uint8
			if mathEnable[lm] /*&& !preventMath*/ {
				if cs == 0 {
					c = cm
				} else if cm == 0 {
					c = cs
				} else {
					// do color math between main and sub:
					key := uint16(cm) | uint16(cs)<<8

					var ok bool
					if c, ok = mixedColors[key]; !ok {
						c = hc
						r1, g1, b1, _ := pal[cm].RGBA()
						r2, g2, b2, _ := pal[cs].RGBA()
						if halve {
							r1 >>= 1
							r2 >>= 1
							g1 >>= 1
							g2 >>= 1
							b1 >>= 1
							b2 >>= 1
						}
						if subtractColors {
							// sub:
							pal[c] = color.RGBA64{
								R: sat(r1 - r2),
								G: sat(g1 - g2),
								B: sat(b1 - b2),
								A: 0xffff,
							}
						} else {
							// add:
							pal[c] = color.RGBA64{
								R: sat(r1 + r2),
								G: sat(g1 + g2),
								B: sat(b1 + b2),
								A: 0xffff,
							}
						}
						mixedColors[key] = c

						// find next unused color:
						usedColors[hc] = true
						for usedColors[hc] {
							hc++
						}
					}
				}
			} else {
				c = cm
				if c == 0 {
					c = cs
				}
			}

			dst.SetColorIndex(x, y, c)
		}
	}

	_ = preventMath
}

func RenderGIF(g *gif.GIF, fname string) {
	// present last frame for 3 seconds:
	f := len(g.Delay) - 1
	if f >= 0 {
		g.Delay[f] = 300
	}

	// render GIF:
	gw, err := os.OpenFile(
		fname,
		os.O_TRUNC|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		panic(err)
	}
	defer gw.Close()

	err = gif.EncodeAll(gw, g)
	if err != nil {
		panic(err)
	}
}

func exportPNG(name string, g image.Image) (err error) {
	// export to PNG:
	var po *os.File

	po, err = os.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() {
		err = po.Close()
		if err != nil {
			return
		}
	}()

	bo := bufio.NewWriterSize(po, 8*1024*1024)

	err = png.Encode(bo, g)
	if err != nil {
		return
	}

	err = bo.Flush()
	if err != nil {
		return
	}

	return
}

var gammaRamp = [...]uint8{
	0x00, 0x01, 0x03, 0x06, 0x0a, 0x0f, 0x15, 0x1c,
	0x24, 0x2d, 0x37, 0x42, 0x4e, 0x5b, 0x69, 0x78,
	0x88, 0x90, 0x98, 0xa0, 0xa8, 0xb0, 0xb8, 0xc0,
	0xc8, 0xd0, 0xd8, 0xe0, 0xe8, 0xf0, 0xf8, 0xff,
}

func cgramRGBA(bgr15 uint16) color.NRGBA {
	// convert BGR15 color format (MSB unused) to RGB24:
	b := (bgr15 & 0x7C00) >> 10
	g := (bgr15 & 0x03E0) >> 5
	r := bgr15 & 0x001F
	if useGammaRamp {
		return color.NRGBA{
			R: gammaRamp[r],
			G: gammaRamp[g],
			B: gammaRamp[b],
			A: 0xff,
		}
	} else {
		return color.NRGBA{
			R: uint8(r<<3 | r>>2),
			G: uint8(g<<3 | g>>2),
			B: uint8(b<<3 | b>>2),
			A: 0xff,
		}
	}
}

func cgramToPalette(cgram []uint16) color.Palette {
	pal := make(color.Palette, 256)
	for i, bgr15 := range cgram {
		pal[i] = cgramRGBA(bgr15)
	}
	return pal
}

func renderBG(g *image.Paletted, bg []uint16, tiles []uint8, prio uint8) {
	a := uint32(0)
	for ty := 0; ty < 64; ty++ {
		for tx := 0; tx < 64; tx++ {
			z := bg[a]
			a++

			// priority check:
			if (z&0x2000 != 0) != (prio != 0) {
				continue
			}

			draw4bppBGTile(g, z, tiles, tx, ty)
		}
	}
}

func renderBGsep(g [2]*image.Paletted, bg []uint16, tiles []uint8, p0 bool, p1 bool) {
	a := uint32(0)
	for ty := 0; ty < 64; ty++ {
		for tx := 0; tx < 64; tx++ {
			z := bg[a]
			a++

			// priority check:
			p := (z & 0x2000) >> 13
			if p == 0 && !p0 {
				continue
			}
			if p == 1 && !p1 {
				continue
			}
			draw4bppBGTile(g[p], z, tiles, tx, ty)
		}
	}
}

func renderVRAMBG(g [2]*image.Paletted, bg []uint16, tiles []uint8, p0 bool, p1 bool) {
	for ty := 0; ty < 64; ty++ {
		for tx := 0; tx < 64; tx++ {
			a := uint32(ty&31)*32 + uint32(tx&31)
			a += (uint32(tx) & 0x20) << 5
			a += (uint32(ty) & 0x20) << 6
			z := bg[a]

			// priority check:
			p := (z & 0x2000) >> 13
			if p == 0 && !p0 {
				continue
			}
			if p == 1 && !p1 {
				continue
			}
			draw4bppBGTile(g[p], z, tiles, tx, ty)
		}
	}
}

func renderMap8(g [2]*image.Paletted, wh int, map8 []uint16, tiles []uint8, p0 bool, p1 bool) {
	// wh == 0x40 for underworld
	// wh == 0x80 for overworld (large area)
	a := uint32(0)
	for ty := (0); ty < wh; ty++ {
		for tx := (0); tx < wh; tx, a = tx+1, a+1 {
			z := map8[a]

			// priority check:
			p := (z & 0x2000) >> 13
			if p == 0 && !p0 {
				continue
			}
			if p == 1 && !p1 {
				continue
			}
			draw4bppBGTile(g[p], z, tiles, tx, ty)
		}
	}
}

func draw4bppBGTile(g *image.Paletted, z uint16, tiles []uint8, tx int, ty int) {
	//High     Low          Legend->  c: Starting character (tile) number
	//vhopppcc cccccccc               h: horizontal flip  v: vertical flip
	//                                p: palette number   o: priority bit

	p := byte((z>>10)&7) << 4
	c := int(z & 0x03FF)
	for y := 0; y < 8; y++ {
		fy := y
		if z&0x8000 != 0 {
			fy = 7 - y
		}
		p0 := tiles[(c<<5)+(y<<1)]
		p1 := tiles[(c<<5)+(y<<1)+1]
		p2 := tiles[(c<<5)+(y<<1)+16]
		p3 := tiles[(c<<5)+(y<<1)+17]
		for x := 0; x < 8; x++ {
			fx := x
			if z&0x4000 == 0 {
				fx = 7 - x
			}

			i := byte((p0>>x)&1) |
				byte(((p1>>x)&1)<<1) |
				byte(((p2>>x)&1)<<2) |
				byte(((p3>>x)&1)<<3)

			// transparency:
			if i == 0 {
				continue
			}

			g.SetColorIndex(tx<<3+fx, ty<<3+fy, p+i)
		}
	}
}
