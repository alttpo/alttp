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

type OWEdge struct {
	absX int
	absY int
	d    Direction
}

func createAreaIfNotExists(t T, loadArea func(T, *System)) (a *Area) {
	var err error

	defer func() {
		if ex := recover(); ex != nil {
			fmt.Printf("ERROR: %v\n%s", ex, string(debug.Stack()))
			a = &Area{
				AreaID:   t.AreaID,
				IsLoaded: false,
			}
		}
	}()

	t.AreasLock.Lock()
	defer t.AreasLock.Unlock()

	var ok bool
	if a, ok = t.Areas[t.AreaID]; !ok {
		e := &System{}
		if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
			panic(err)
		}

		fmt.Printf("%s: load\n", t.AreaID)
		loadArea(t, e)

		a = createArea(t, e)
		t.Areas[t.AreaID] = a
	}

	return
}

func ReachTaskOverworldFromUnderworldWorker(q Q, t T) {
	var err error

	var a *Area
	a = createAreaIfNotExists(
		t,
		func(t T, e *System) {

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
		},
	)

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
	t.OWSS = OWSS{
		c: OWCoord(((t.Y-ay)>>3+6)<<7 + (t.X-ax)>>3),
		d: DirSouth,
	}

	a.overworldFloodFill(q, t)
}

func ReachTaskOverworldEdgeWorker(q Q, t T) {
	var err error

	fmt.Printf("%s: overworld edge worker!\n", t.AreaID)

	var a *Area
	a = createAreaIfNotExists(
		t,
		func(t T, e *System) {
			copy(e.WRAM[:], t.EntranceWRAM[:])
			copy(e.VRAM[:], t.EntranceVRAM[:])

			wram := (*e.WRAM)[:]

			// if uint16(st) == 0xA7 {
			// 	e.LoggerCPU = os.Stdout
			// }

			// set the AreaID to load and direction to transition from:
			write8(wram, 0x8A, uint8(t.AreaID))
			write8(wram, 0x040A, uint8(t.AreaID))
			db := uint8(1 << (3 - t.OWSS.d))
			write8(wram, 0x0410, db)
			write8(wram, 0x0416, db)
			write8(wram, 0x0418, uint8(t.OWSS.d))
			write8(wram, 0x069C, uint8(t.OWSS.d))
			if err = e.ExecAt(b02LoadOverworldTransitionPC, donePC); err != nil {
				panic(err)
			}

			// run frames until back to module $09:
			for i := 0; i < 256; i++ {
				if err = e.ExecAt(b00RunSingleFramePC, donePC); err != nil {
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
		},
	)

	if !a.IsLoaded {
		return
	}

	// wram := a.WRAM[:]

	for _, ed := range t.OWEdges {
		row, col := ed.absY, ed.absX

		// offset from area top-left:
		arow, acol := a.AreaID.RowCol()
		col -= (acol * 0x40)
		row -= (arow * 0x40)

		// adjust to size:
		col &= a.Width - 1
		row &= a.Height - 1

		t.OWSS.c = RowColToOWCoord(row, col)
		t.OWSS.d = ed.d

		a.overworldFloodFill(q, t)
	}
}

func ReachTaskOverworldWarpWorker(q Q, t T) {
	var err error

	fmt.Printf("%s: overworld warp worker!\n", t.AreaID)

	var a *Area
	a = createAreaIfNotExists(
		t,
		func(t T, e *System) {
			copy(e.WRAM[:], t.EntranceWRAM[:])
			copy(e.VRAM[:], t.EntranceVRAM[:])

			wram := (*e.WRAM)[:]

			// if uint16(st) == 0xA7 {
			// 	e.LoggerCPU = os.Stdout
			// }

			if read8(wram, 0x10) != 0x09 {
				panic("expected module $09")
			}
			// move to warp submodule (from LW to DW):
			write8(wram, 0x11, 0x23)

			// take the first warp coord and move Link there:
			c := t.OWWarps[0]
			linkX, linkY := t.AreaID.AbsXY(c)
			write16(wram, 0x22, uint16(linkX))
			write16(wram, 0x20, uint16(linkY))

			// run frames until back to module $09:
			for i := 0; i < 512; i++ {
				if err = e.ExecAt(b00RunSingleFramePC, donePC); err != nil {
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

			// verify module, submodule:
			if read8(wram, 0x10) != 0x09 {
				panic("expected module $09")
			}
			if read8(wram, 0x11) != 0x00 {
				panic("expected submodule $00")
			}

			// verify we're in the appropriate world:
			if got, expected := AreaID(read8(wram, 0x8A)), t.AreaID; got != expected {
				panic(fmt.Sprintf("expected areaID to be %02X, got %02X\n", uint8(expected), uint8(got)))
			}
			// e.LoggerCPU = nil
		},
	)

	if !a.IsLoaded {
		return
	}

	// wram := a.WRAM[:]

	for _, c := range t.OWWarps {
		// pick arbitrary direction; doesn't matter much:
		t.OWSS.c = c
		t.OWSS.d = DirSouth

		a.overworldFloodFill(q, t)
	}
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
	copy(a.VRAMAfterLoaded[:], (*e.VRAM)[:])
	copy(a.VRAMTileSet[:], (*e.VRAM)[0x4000:0x8000])

	// point emulator WRAM at Area's copy:
	e.WRAM = &a.WRAM

	wram := (*e.WRAM)[:]

	// grab area width,height extents in tiles:
	a.Height = int(read16(wram, 0x070A)+0x10) >> 3
	a.Width = int(read16(wram, 0x070E) + 0x02)

	ah := uint32(a.Height)
	aw := uint32(a.Width)

	// find overworld tile secrets and reveal them:
	{
		j := uint32(0x1B_0000) | uint32(e.Bus.Read16(alttp.OverworldData_HiddenItems+(uint32(a.AreaID)<<1)))
		for ; ; j += 3 {
			// location of secret:
			m16 := e.Bus.Read16(j + 0)
			if m16 == 0xFFFF {
				break
			}

			// type of secret:
			v := e.Bus.Read8(j + 2)
			if v < 0x80 {
				// not a tile replacement:
				continue
			}
			if v == 0x84 {
				// stairs are broken in this lookup for some reason:
				continue
			}

			// find the tile type:
			t16 := e.Bus.Read16(alttp.Overworld_SecretTileType + uint32(v&0x0F))
			fmt.Printf("%s: secret reveal at %04X: %04X\n", a.AreaID, m16, t16)
			// do the tile replacement:
			write16(wram[0x2000:], uint32(m16), t16)
		}
	}

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
		aid := AreaID(e.Bus.Read16(alttp.Overworld_EntranceScreens + j<<1))
		if aid != a.AreaID {
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
		aid := AreaID(e.Bus.Read16(alttp.Overworld_GetPitDestination_screen + j<<1))
		if aid != a.AreaID {
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

	for i := range a.Map8 {
		c := OWCoord(i)
		row, col := a.RowCol(c)
		if col >= int(a.Width)-4 {
			continue
		}
		if row >= int(a.Height)-4 {
			continue
		}

		// open doors:
		v00 := a.Map8[c+0] & 0x41FF
		v01 := a.Map8[c+1] & 0x41FF
		v02 := a.Map8[c+2] & 0x41FF
		v03 := a.Map8[c+3] & 0x41FF
		if v00 == 0x0148 && v01 == 0x0149 && v02 == 0x4149 && v03 == 0x4148 {
			// 1D48 1D49 5D49 5D48
			// open castle door:
			for j := 0; j < 4; j++ {
				a.Tiles[c+0x000+OWCoord(j)] = 0x00
				a.Tiles[c+0x080+OWCoord(j)] = 0x00
				a.Tiles[c+0x100+OWCoord(j)] = 0x00
			}
		}
		if v00 == 0x00E8 && v01 == 0x00E9 && v02 == 0x40E9 && v03 == 0x40E8 {
			// 08E8 08E9 48E9 48E8
			// at top of regular door: (e.g. village house)
			for j := 0; j < 2; j++ {
				a.Tiles[c+0x001+OWCoord(j)] = 0x00
				a.Tiles[c+0x081+OWCoord(j)] = 0x00
			}
		}

		// convert hammer pegs:
		v10 := a.Map8[c+0x80] & 0x41FF
		v11 := a.Map8[c+0x81] & 0x41FF
		// 19A0 59A0
		// 19B0 59B0
		if v00 == 0x01A0 && v01 == 0x41A0 && v10 == 0x01B0 && v11 == 0x41B0 {
			for j := 0; j < 2; j++ {
				a.Tiles[c+0x000+OWCoord(j)] = 0x70
				a.Tiles[c+0x080+OWCoord(j)] = 0x70
			}
		}
	}
	// TODO: open GT stairs
	// TODO: open TR entrance
	// TODO: open MM entrance
	// TODO: open TT entrance
	// TODO: why no entrance 0A for DP??

	if true {
		os.WriteFile(
			fmt.Sprintf("ow%02X.map16", uint8(a.AreaID)),
			(*(*[0x80 * 0x80]byte)(unsafe.Pointer(&wram[0x2000])))[:],
			0644,
		)
		os.WriteFile(
			fmt.Sprintf("ow%02X.map8", uint8(a.AreaID)),
			(*(*[0x80 * 0x80 * 2]byte)(unsafe.Pointer(&a.Map8[0])))[:],
			0644,
		)
		os.WriteFile(
			fmt.Sprintf("ow%02X.tmap", uint8(a.AreaID)),
			a.Tiles[:],
			0644,
		)
	}

	return
}

func (a *Area) overworldFloodFill(q Q, t T) {
	a.Mutex.Lock()
	defer a.Mutex.Unlock()

	fmt.Printf(
		"%s: start %04X %5s\n",
		a.AreaID,
		uint16(t.OWSS.c),
		t.OWSS.d,
	)

	lifo := make([]OWSS, 0, 0x1000)
	lifo = append(lifo, t.OWSS)

	m := a.Tiles[:]

	areaEdges := map[AreaID][]OWEdge{}
	warps := map[AreaID][]OWCoord{}

	for len(lifo) > 0 {
		s := lifo[len(lifo)-1]
		lifo = lifo[:len(lifo)-1]

		c := s.c
		d := s.d
		cn := c
		switch d {
		case DirNorth:
			cn, _, _ = a.Traverse(c, DirEast, 1)
		case DirSouth:
			cn, _, _ = a.Traverse(c, DirEast, 1)
		case DirWest:
			cn, _, _ = a.Traverse(c, DirSouth, 1)
		case DirEast:
			cn, _, _ = a.Traverse(c, DirSouth, 1)
		}

		if _, ok := a.TilesVisited[c]; ok {
			continue
		}
		a.TilesVisited[c] = empty{}

		canTraverse := false
		canTurn := false

		v, vn := m[c], m[cn]
		if v == 0x20 {
			// pit:
			a.Reachable[c] = v
			a.Reachable[cn] = v
		} else if v == 0x08 {
			// deep water:
			canTraverse = true
			canTurn = true
		} else if v == 0x52 || v == 0x53 {
			// gray rock and black rock:
			canTraverse = true
			canTurn = true
		} else if v == 0x55 || v == 0x56 {
			// large gray rock and large black rock:
			canTraverse = true
			canTurn = true
		} else if v == 0x57 || v == 0x57 {
			// bonk rocks:
			canTraverse = true
			canTurn = true
		} else if a.isAlwaysWalkable(v) && a.isAlwaysWalkable(vn) {
			canTraverse = true
			canTurn = true

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

		if vt, ok := a.GetMap16At(c); ok {
			switch vt {
			case 0x0212: // warp:
				if a.AreaID&0x40 == 0x40 {
					panic("cannot warp from DW!")
				}
				// set DW bit:
				na := a.AreaID | 0x40
				// queue up a warp:
				warps[na] = append(warps[na], c)
			}
		}

		// TODO: mirroring from DW to LW
		// TODO: jumping off ledges into pits
		// TODO: hookshot across DM bridge

		if !canTraverse {
			continue
		}

		a.Reachable[c] = v
		a.Reachable[cn] = v

		// transition to neighboring area at the edges:
		if absX, absY, na, ok := a.NeighborEdge(c, d); ok {
			fmt.Printf("%s: edge $%04X %s exit to %s starting at (%03X,%03X)\n", t.AreaID, uint16(c), d, na, absX, absY)
			areaEdges[na] = append(areaEdges[na], OWEdge{absX: absX, absY: absY, d: d})
			continue
		}

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

	// submit one task for each area-edge pair:
	for na, el := range areaEdges {
		fmt.Printf("%s: submit overworld edge task for %s with %d edges\n", a.AreaID, na, len(el))
		// correct the AreaID to account for large areas:
		q.SubmitTask(&ReachTask{
			Mode:            ModeOverworld,
			Rooms:           t.Rooms,
			RoomsLock:       t.RoomsLock,
			Areas:           t.Areas,
			AreasLock:       t.AreasLock,
			InitialEmulator: t.InitialEmulator,

			EntranceWRAM: &a.WRAMAfterLoaded,
			EntranceVRAM: &a.VRAMAfterLoaded,

			AreaID:  na,
			OWEdges: el,
		}, ReachTaskOverworldEdgeWorker)
	}

	for na, wl := range warps {
		fmt.Printf("%s: submit overworld warp task for %s with %d warps\n", a.AreaID, na, len(wl))
		// correct the AreaID to account for large areas:
		q.SubmitTask(&ReachTask{
			Mode:            ModeOverworld,
			Rooms:           t.Rooms,
			RoomsLock:       t.RoomsLock,
			Areas:           t.Areas,
			AreasLock:       t.AreasLock,
			InitialEmulator: t.InitialEmulator,

			EntranceWRAM: &a.WRAMAfterLoaded,
			EntranceVRAM: &a.VRAMAfterLoaded,

			AreaID:  na,
			OWWarps: wl,
		}, ReachTaskOverworldWarpWorker)
	}
}
