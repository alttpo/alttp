package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"unsafe"
)

type OWSS struct {
	c OWCoord
	d Direction
}

func ReachTaskOverworldFromUnderworldWorker(q Q, t T) {
	var err error

	var a *Area
	var ok bool

	t.AreasLock.Lock()
	if a, ok = t.Areas[t.AreaID]; !ok {
		e := &System{}
		if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
			panic(err)
		}

		fmt.Printf("%s: load\n", t.AreaID)
		func() {
			defer func() {
				if ex := recover(); ex != nil {
					fmt.Printf("ERROR: %v\n%s", ex, string(debug.Stack()))
					a = &Area{
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

			a = createArea(t, e)
		}()
		t.Areas[t.AreaID] = a
	}
	t.AreasLock.Unlock()

	if !a.IsLoaded {
		return
	}

	wram := a.WRAM[:]

	ax := read16(wram, 0x070C) << 3
	ay := read16(wram, 0x0708)

	fmt.Printf(
		"%s: area at abs %04X, %04X\n",
		a.AreaID,
		ax,
		ay,
	)

	fmt.Printf(
		"%s: exit at abs %04X, %04X\n",
		a.AreaID,
		uint16(t.X),
		uint16(t.Y),
	)

	// linkX := read16(wram, 0x22)
	// linkY := read16(wram, 0x20)
	// fmt.Printf(
	// 	"%s: link at abs %04X, %04X\n",
	// 	a.AreaID,
	// 	linkX,
	// 	linkY,
	// )

	// fmt.Printf(
	// 	"%s: link at rel %04X, %04X\n",
	// 	a.AreaID,
	// 	linkX-ax,
	// 	linkY-ay,
	// )

	// set up initial scan state at where Link is:
	// se := OWSS{
	// 	c: OWCoord(((linkY-ay)>>3)<<7 + (linkX-ax)>>3),
	// 	d: DirSouth,
	// }
	se := OWSS{
		c: OWCoord(((t.Y-ay)>>3+6)<<7 + (t.X-ax)>>3),
		d: DirSouth,
	}

	t.OWStates = append(t.OWStates, se)

	fmt.Printf(
		"%s: start %04X\n",
		a.AreaID,
		uint16(se.c),
	)

	a.overworldFloodFill(q, t)
}

func createArea(t T, e *System) (a *Area) {
	a = &Area{
		AreaID:          t.AreaID,
		Width:           0,
		Height:          0,
		IsLoaded:        true,
		Rendered:        nil,
		RenderedNRGBA:   nil,
		TilesVisited:    map[OWCoord]empty{},
		Tiles:           [0x4000]byte{},
		Reachable:       [0x4000]byte{},
		Hookshot:        [0x4000]byte{},
		AllowDirFlags:   [0x4000]uint8{},
		e:               e,
		WRAM:            [131072]byte{},
		WRAMAfterLoaded: [131072]byte{},
		VRAMTileSet:     [0x4000]byte{},
		TileEntrance:    map[OWCoord]*AreaEntrance{},
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
			m16 := uint32(read16(wram[0x2000:], (row*0x40+col))) << 3

			// translate into map8 blocks via Map16Definitions:
			df := [4]uint16{
				e.Bus.Read16(alttp.Map16Definitions + (m16 + 0)),
				e.Bus.Read16(alttp.Map16Definitions + (m16 + 2)),
				e.Bus.Read16(alttp.Map16Definitions + (m16 + 4)),
				e.Bus.Read16(alttp.Map16Definitions + (m16 + 6)),
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

	for i := range a.Reachable {
		a.Reachable[i] = 0x01
	}

	// find all overworld entrances to underworld:
	ec := alttp.Overworld_EntranceCount
	for j := uint32(0); j < ec; j++ {
		aid := e.Bus.Read16(alttp.Overworld_EntranceScreens + j<<1)
		if aid != uint16(a.AreaID) {
			continue
		}

		m16pos := e.Bus.Read16(alttp.Overworld_EntranceScreens + (ec * 2) + j<<1)
		ent := AreaEntrance{
			OWCoord:    Map16ToOWCoord(m16pos),
			EntranceID: e.Bus.Read8(alttp.Overworld_EntranceScreens + (ec * 4) + j),
		}
		a.Entrances = append(a.Entrances, ent)
		i := len(a.Entrances) - 1

		fmt.Printf("%s: entrance at map16=%04X, id=%02X at %04X\n", a.AreaID, m16pos, ent.EntranceID, uint16(ent.OWCoord))

		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				a.TileEntrance[ent.OWCoord+OWCoord(y*0x80)+OWCoord(x)] = &a.Entrances[i]
			}
		}
	}

	// add pit entrances:
	for j := uint32(0); j < alttp.Overworld_GetPitDestination_count; j++ {
		aid := e.Bus.Read16(alttp.Overworld_GetPitDestination_screen + j<<1)
		if aid != uint16(a.AreaID) {
			continue
		}

		// i don't understand the reason for the 0x400 fudge factor but it's definitely needed.
		m16pos := e.Bus.Read16(alttp.Overworld_GetPitDestination_map16+j<<1) + 0x400
		ent := AreaEntrance{
			OWCoord:    OWCoord((m16pos & 0x7F) | ((m16pos >> 7) << 8)),
			EntranceID: e.Bus.Read8(alttp.Overworld_GetPitDestination_entrance + j),
			IsPit:      true,
		}
		a.Entrances = append(a.Entrances, ent)
		i := len(a.Entrances) - 1

		fmt.Printf("%s: pit entrance at map16=%04X, id=%02X at %04X\n", a.AreaID, m16pos, ent.EntranceID, uint16(ent.OWCoord))

		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				a.TileEntrance[ent.OWCoord+OWCoord(y*0x80)+OWCoord(x)] = &a.Entrances[i]
			}
		}
	}

	for _, ent := range a.Entrances {
		fmt.Printf("%s: entrance id=%02X at %04X\n", a.AreaID, ent.EntranceID, uint16(ent.OWCoord))
	}

	// open doors:
	for i := range a.Map8 {
		c := OWCoord(i)
		row, col := a.RowCol(c)
		if col >= int(a.Width)-3 {
			continue
		}
		if row >= int(a.Height)-4 {
			continue
		}

		v0 := a.Map8[c+0] & 0x41FF
		v1 := a.Map8[c+1] & 0x41FF
		v2 := a.Map8[c+2] & 0x41FF
		v3 := a.Map8[c+3] & 0x41FF
		if v0 == 0x0148 && v1 == 0x0149 && v2 == 0x4149 && v3 == 0x4148 {
			// 1D48 1D49 5D49 5D48
			// open castle door:
			a.ClearMap8Tile(c + 0x000)
			a.ClearMap8Tile(c + 0x080)
			a.ClearMap8Tile(c + 0x100)
		}
		if v0 == 0x00E9 && v1 == 0x40E9 {
			// at top of regular door: (e.g. village house)
			a.ClearMap8Tile(c)
		}
	}

	if true {
		os.WriteFile(
			fmt.Sprintf("ow%02X.map16", a.AreaID),
			(*(*[0x80 * 0x80]byte)(unsafe.Pointer(&wram[0x2000])))[:],
			0644,
		)
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

	return
}

func (a *Area) overworldFloodFill(q Q, t T) {
	a.Mutex.Lock()
	defer a.Mutex.Unlock()

	lifo := make([]OWSS, 0, 0x1000)
	lifo = append(lifo, t.OWStates...)

	m := a.Tiles[:]

	for len(lifo) > 0 {
		s := lifo[len(lifo)-1]
		lifo = lifo[:len(lifo)-1]

		c := s.c
		d := s.d

		if _, ok := a.TilesVisited[c]; ok {
			continue
		}
		a.TilesVisited[c] = empty{}

		canTraverse := false
		canTurn := false

		v := m[c]
		if v == 0x20 {
			// pit:
			a.Reachable[c] = v
		} else if a.isAlwaysWalkable(v) {
			canTraverse = true
			canTurn = true
			a.Reachable[c] = v

			if ent, ok := a.TileEntrance[c]; ok {
				fmt.Printf("%s: underworld entrance %02X at %04X\n", a.AreaID, ent.EntranceID, uint16(c))
				if !ent.Used {
					ent.Used = true
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
		}

		if canTraverse {
			if canTurn {
				if c, d, ok := a.Traverse(c, d.RotateCCW(), 1); ok {
					lifo = append(lifo, OWSS{c: c, d: d})
				}
				if c, d, ok := a.Traverse(c, d.RotateCW(), 1); ok {
					lifo = append(lifo, OWSS{c: c, d: d})
				}
			}
			if c, d, ok := a.Traverse(c, d, 1); ok {
				lifo = append(lifo, OWSS{c: c, d: d})
			}
		}
	}
}
