package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"unsafe"
)

func ReachTaskOverworldFromUnderworldWorker(q Q, t T) {
	var err error

	var area *Area
	var ok bool

	t.AreasLock.Lock()
	if area, ok = t.Areas[t.AreaID]; !ok {
		e := &System{}
		if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
			panic(err)
		}

		fmt.Printf("OW$%02X: load\n", t.AreaID)
		func() {
			defer func() {
				if ex := recover(); ex != nil {
					fmt.Printf("ERROR: %v\n%s", ex, string(debug.Stack()))
					area = &Area{
						AreaID:   t.AreaID,
						IsLoaded: false,
					}
				}
			}()

			copy(e.WRAM[:], t.EntranceWRAM[:])
			copy(e.VRAM[:], t.EntranceVRAM[:])

			wram := (*e.WRAM)[:]

			// if uint16(st) == 0xA7 {
			// 	e.LoggerCPU = os.Stdout
			// }

			// load module $08 to transition from underworld to overworld:
			// note: this should automatically detect the only custom exit in the room.
			write8(wram, 0x10, 0x08)
			write8(wram, 0x11, 0x00)
			// run frames until back to module $09:
			for i := 0; i < 256; i++ {
				if err = e.ExecAt(runFramePC, donePC); err != nil {
					panic(err)
				}

				// f++
				// fmt.Printf(
				// 	"f%04d: %02X %02X %02X\n",
				// 	f,
				// 	read8(wram, 0x010),
				// 	read8(wram, 0x011),
				// 	read8(wram, 0x0B0),
				// )

				// wait until module 09 or 0B (overworld):
				if m := read8(wram, 0x10); m == 0x09 || m == 0x0B {
					// wait until submodule goes back to 0:
					if read8(wram, 0x011) == 0x00 {
						break
					}
				}
			}
			// e.LoggerCPU = nil

			area = createArea(t, e)
		}()
		t.Areas[t.AreaID] = area
	}
	t.AreasLock.Unlock()

	if !area.IsLoaded {
		return
	}

	area.overworldFloodFill(q, t)
}

func createArea(t T, e *System) (a *Area) {
	a = &Area{
		AreaID:          t.AreaID,
		Width:           0,
		Height:          0,
		IsLoaded:        true,
		Rendered:        nil,
		RenderedNRGBA:   nil,
		TilesVisited:    map[MapCoord]struct{}{},
		Tiles:           [16384]byte{},
		Reachable:       [16384]byte{},
		Hookshot:        map[MapCoord]byte{},
		AllowDirFlags:   [16384]uint8{},
		e:               e,
		WRAM:            [131072]byte{},
		WRAMAfterLoaded: [131072]byte{},
		VRAMTileSet:     [16384]byte{},
	}

	copy(a.WRAM[:], (*e.WRAM)[:])
	copy(a.WRAMAfterLoaded[:], (*e.WRAM)[:])
	copy(a.VRAMTileSet[:], (*e.VRAM)[0x4000:0x8000])

	// point emulator WRAM at Area's copy:
	e.WRAM = &a.WRAM

	wram := (*e.WRAM)[:]

	// grab area width,height extents in tiles:
	a.Height = (read16(wram, 0x070A) + 0x10) >> 3
	a.Width = (read16(wram, 0x070E) + 0x02)

	ah := uint32(a.Height)
	aw := uint32(a.Width)

	// decode map16 overworld from $7E2000 into both map8 and tile types:
	for row := uint32(0); row < ah; row += 2 {
		for col := uint32(0); col < aw; col += 2 {
			// read map16 blocks from WRAM at $7E2000:
			m16 := uint32(read16(wram[0x2000:], (row*0x40 + col)))

			// translate into map8 blocks via Map16Definitions:
			df := [4]uint16{
				e.Bus.Read16(alttp.Map16Definitions + (m16 << 3) + 0),
				e.Bus.Read16(alttp.Map16Definitions + (m16 << 3) + 2),
				e.Bus.Read16(alttp.Map16Definitions + (m16 << 3) + 4),
				e.Bus.Read16(alttp.Map16Definitions + (m16 << 3) + 6),
			}

			// store map8 blocks:
			a.Map8[((row+0)*0x80)+(col+0)] = df[0]
			a.Map8[((row+0)*0x80)+(col+1)] = df[1]
			a.Map8[((row+1)*0x80)+(col+0)] = df[2]
			a.Map8[((row+1)*0x80)+(col+1)] = df[3]

			// translate presentation map8 blocks into tile types:
			a.Tiles[((row+0)*0x80)+(col+0)] = e.Bus.Read8(alttp.OverworldTileTypes + uint32(df[0]&0x01FF))
			a.Tiles[((row+0)*0x80)+(col+1)] = e.Bus.Read8(alttp.OverworldTileTypes + uint32(df[1]&0x01FF))
			a.Tiles[((row+1)*0x80)+(col+0)] = e.Bus.Read8(alttp.OverworldTileTypes + uint32(df[2]&0x01FF))
			a.Tiles[((row+1)*0x80)+(col+1)] = e.Bus.Read8(alttp.OverworldTileTypes + uint32(df[3]&0x01FF))
		}
	}

	// find all overworld entrances to underworld:
	ec := alttp.Overworld_EntranceCount
	for j := uint32(0); j < ec; j++ {
		aid := e.Bus.Read16(alttp.Overworld_EntranceScreens + j<<1)
		if aid != uint16(a.AreaID) {
			continue
		}

		ent := AreaEntrance{
			TileIndex:  e.Bus.Read16(alttp.Overworld_EntranceScreens + (ec * 2) + j<<1),
			EntranceID: e.Bus.Read8(alttp.Overworld_EntranceScreens + (ec * 4) + j),
		}
		a.Entrances = append(a.Entrances, ent)
		fmt.Printf("OW$%02X: entrance id=%02X at %04X\n", a.AreaID, ent.EntranceID, ent.TileIndex)
	}

	if true {
		os.WriteFile(
			fmt.Sprintf("ow%02X.map8", a.AreaID),
			(*(*[0x80 * 0x80 * 2]byte)(unsafe.Pointer(&a.Map8[0])))[:],
			0644,
		)
		os.WriteFile(
			fmt.Sprintf("ow%02X.tmap", a.AreaID),
			a.Tiles[:],
			0644,
		)
	}

	a.Render()
	exportPNG(fmt.Sprintf("ow%02X.png", a.AreaID), a.RenderedNRGBA)

	return
}

func (a *Area) overworldFloodFill(q Q, t T) {
	fmt.Println("TODO: overworldFloodFill")

	for _, ent := range a.Entrances {
		// TODO: assume entrances to underworld are reachable
		q.SubmitTask(&ReachTask{
			Mode:            ModeUnderworld,
			Rooms:           t.Rooms,
			RoomsLock:       t.RoomsLock,
			Areas:           t.Areas,
			AreasLock:       t.AreasLock,
			InitialEmulator: t.InitialEmulator,
			EntranceID:      ent.EntranceID,
		}, ReachTaskFromEntranceWorker)
	}
}
