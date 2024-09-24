package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/gif"
	"roomloader/taskqueue"
	"strings"
	"sync"

	"golang.org/x/image/math/fixed"
)

type SE struct {
	c    MapCoord
	d    Direction
	s    int
	wram *WRAMArray
}

type ReachTask struct {
	InitialEmulator *System
	EntranceWRAM    *WRAMArray
	EntranceVRAM    *VRAMArray

	Rooms     map[uint16]*RoomState
	RoomsLock *sync.Mutex

	EntranceID uint8
	Supertile  Supertile

	SE SE
}

type T = *ReachTask
type Q = *taskqueue.Q[T]

func ReachTaskInterRoom(q Q, t T) {
	var err error

	var room *RoomState
	var ok bool

	t.RoomsLock.Lock()
	if room, ok = t.Rooms[uint16(t.Supertile)]; !ok {
		e := &System{}
		if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
			panic(err)
		}

		// restore previous WRAM+VRAM pair:
		if t.EntranceWRAM != nil {
			copy((*e.WRAM)[:], (*t.EntranceWRAM)[:])
		}
		if t.EntranceVRAM != nil {
			copy((*e.VRAM)[:], (*t.EntranceVRAM)[:])
		}

		st := t.Supertile
		wram := (e.WRAM)[:]

		// load and draw current supertile:
		write16(wram, 0xA0, uint16(st))
		write16(wram, 0x048E, uint16(st))

		//e.LoggerCPU = e.Logger
		if err = e.ExecAt(loadSupertilePC, donePC); err != nil {
			panic(err)
		}
		//e.LoggerCPU = nil

		room = createRoom(t, e)
		t.Rooms[uint16(t.Supertile)] = room
	}
	t.RoomsLock.Unlock()

	reachTaskFloodfill(q, t, room)

	room.RenderToNonPaletted()
}

func ReachTaskFromEntranceWorker(q Q, t T) {
	var err error

	e := &System{}
	if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
		panic(err)
	}

	// poke the entrance ID into our asm code:
	e.HWIO.Dyn[setEntranceIDPC&0xffff-0x5000] = t.EntranceID

	// load the entrance:
	if err = e.ExecAt(loadEntrancePC, donePC); err != nil {
		panic(err)
	}

	wram := (e.WRAM)[:]

	t.EntranceWRAM = new(WRAMArray)
	copy((*t.EntranceWRAM)[:], (*e.WRAM)[:])
	t.EntranceVRAM = new(VRAMArray)
	copy((*t.EntranceVRAM)[:], (*e.VRAM)[:])

	// read dungeon ID:
	// dungeonID := read8(wram, 0x040C)

	// read initial entrance supertile:
	t.Supertile = Supertile(read16(wram, 0xA0))

	fmt.Printf("entrance $%02X -> $%03X\n", t.EntranceID, uint16(t.Supertile))

	// find Link's entry point:
	linkY := read16(wram, 0x0020)
	linkX := read16(wram, 0x0022)
	linkL := read16(wram, 0x00EE)
	linkC := AbsToMapCoord(linkX, linkY, linkL)
	linkD := Direction(read8(wram, 0x002F) >> 1)

	t.SE.c = linkC
	t.SE.d = linkD
	t.SE.s = 0

	room := getOrCreateRoom(t, e)

	reachTaskFloodfill(q, t, room)

	room.RenderToNonPaletted()

	// outline Link's starting position with entranceID
	drawOutlineBox(
		room.RenderedNRGBA,
		image.NewUniform(color.RGBA{255, 255, 0, 255}),
		int(t.SE.c.Col())*8,
		int(t.SE.c.Row())*8,
		16,
		16,
	)
	drawShadowedString(
		room.RenderedNRGBA,
		image.NewUniform(color.RGBA{255, 255, 0, 255}),
		fixed.Point26_6{X: fixed.I(int(t.SE.c.Col())*8 + 1), Y: fixed.I(int(t.SE.c.Row())*8 + 14)},
		fmt.Sprintf("%02X", uint8(t.EntranceID)),
	)
}

func getOrCreateRoom(t T, e *System) (room *RoomState) {
	t.RoomsLock.Lock()
	defer t.RoomsLock.Unlock()

	var ok bool
	room, ok = t.Rooms[uint16(t.Supertile)]
	if ok {
		return
	}

	room = createRoom(t, e)
	t.Rooms[uint16(t.Supertile)] = room

	return
}

func createRoom(t T, e *System) (room *RoomState) {
	// create new room:
	room = &RoomState{
		Supertile: t.Supertile,
		Entrance: &Entrance{
			EntranceID: t.EntranceID,
			Supertile:  t.Supertile,
			EntryCoord: 0,
			// Rooms:          []*RoomState{},
			// Supertiles:     map[Supertile]*RoomState{},
			// SupertilesLock: sync.Mutex{},
		},
		IsLoaded:         false,
		Rendered:         nil,
		GIF:              gif.GIF{},
		Animated:         gif.GIF{},
		AnimatedTileMap:  [][16384]byte{},
		AnimatedLayers:   []int{},
		AnimatedLayer:    0,
		EnemyMovementGIF: gif.GIF{},
		EntryPoints:      []EntryPoint{},
		ExitPoints:       []ExitPoint{},
		WarpExitTo:       0,
		StairExitTo:      [4]Supertile{},
		WarpExitLayer:    0,
		StairTargetLayer: [4]MapCoord{},
		Doors:            []Door{},
		Stairs:           []MapCoord{},
		SwapLayers:       make(map[MapCoord]struct{}, 0x2000),
		TilesVisitedHash: make(map[uint64]map[MapCoord]struct{}),
		Tiles:            [8192]byte{},
		Reachable:        [8192]byte{},
		Hookshot:         map[MapCoord]byte{},
		e:                e,
		WRAM:             [131072]byte{},
		WRAMAfterLoaded:  [131072]byte{},
		VRAMTileSet:      [16384]byte{},
		markedPit:        false,
		markedFloor:      false,
		lifoSpace:        [8192]ScanState{},
		lifo:             []ScanState{},
		HasReachablePit:  false,
	}

	// do first-time room processing work:
	// fmt.Printf("$%03X: room init\n", uint16(t.Supertile))

	// copy emulator WRAM into room:
	copy(room.WRAM[:], (*e.WRAM)[:])
	copy(room.VRAMTileSet[:], (*e.VRAM)[0x4000:0x8000])

	// point the emulator WRAM at the RoomState's WRAM:
	e.WRAM = &room.WRAM

	wram := (room.WRAM)[:]
	tiles := wram[0x12000:0x14000]

	for i := range room.Reachable {
		room.Reachable[i] = 0x01
		room.AllowDirFlags[i] = tileAllowableDirFlags(tiles[i])
	}

	room.WarpExitTo = Supertile(read8(wram, 0xC000)) | (t.Supertile & 0x100)
	room.StairExitTo = [4]Supertile{
		Supertile(read8(wram, uint32(0xC001))) | (t.Supertile & 0x100),
		Supertile(read8(wram, uint32(0xC002))) | (t.Supertile & 0x100),
		Supertile(read8(wram, uint32(0xC003))) | (t.Supertile & 0x100),
		Supertile(read8(wram, uint32(0xC004))) | (t.Supertile & 0x100),
	}
	room.WarpExitLayer = MapCoord(read8(wram, uint32(0x063C))&2) << 11
	room.StairTargetLayer = [4]MapCoord{
		MapCoord(read8(wram, uint32(0x063D))&2) << 11,
		MapCoord(read8(wram, uint32(0x063E))&2) << 11,
		MapCoord(read8(wram, uint32(0x063F))&2) << 11,
		MapCoord(read8(wram, uint32(0x0640))&2) << 11,
	}

	// os.WriteFile(fmt.Sprintf("r%03X.pre.tmap", uint16(t.Supertile)), tiles, 0644)

	// open up "locked" cell doors:
	for i := uint32(0); i < 6; i++ {
		gt := read16(wram, 0x06E0+i<<1)
		if gt == 0 {
			break
		}

		if gt&0x8000 != 0 {
			// locked cell door:
			t := MapCoord((gt & 0x7FFF) >> 1)
			if tiles[t] == 0x58+uint8(i) {
				tiles[t+0x00] = 0x00
				tiles[t+0x01] = 0x00
				tiles[t+0x40] = 0x00
				tiles[t+0x41] = 0x00
				room.AllowDirFlags[t+0x00] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x01] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x40] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x41] = tileAllowableDirFlags(0)
			}
			if tiles[t|0x1000] == 0x58+uint8(i) {
				tiles[t|0x1000+0x00] = 0x00
				tiles[t|0x1000+0x01] = 0x00
				tiles[t|0x1000+0x40] = 0x00
				tiles[t|0x1000+0x41] = 0x00
				room.AllowDirFlags[t+0x00] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x01] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x40] = tileAllowableDirFlags(0)
				room.AllowDirFlags[t+0x41] = tileAllowableDirFlags(0)
			}
		}
	}

	// open up doorways:
	room.Doors = make([]Door, 0, 16)
	for m := 0; m < 16; m++ {
		tpos := read16(wram, uint32(0x19A0+(m<<1)))
		// stop marker:
		if tpos == 0 {
			break
		}

		door := Door{
			Pos:  MapCoord(tpos >> 1),
			Type: DoorType(read16(wram, uint32(0x1980+(m<<1)))),
			Dir:  Direction(read16(wram, uint32(0x19C0+(m<<1)))),
		}
		room.Doors = append(room.Doors, door)

		if door.Type == 0x30 {
			// exploding wall:
			pos := int(door.Pos)
			for c := 0; c < 11; c++ {
				for r := 0; r < 12; r++ {
					tiles[pos+(r<<6)-c] = 0
					tiles[pos+(r<<6)+1+c] = 0
					room.AllowDirFlags[pos+(r<<6)-c] = tileAllowableDirFlags(0)
					room.AllowDirFlags[pos+(r<<6)+1+c] = tileAllowableDirFlags(0)
				}
			}
			continue
		}

		var ok bool
		var c MapCoord

		// find the first doorway tile:
		var secondTileOffs MapCoord
		switch door.Dir {
		case DirNorth:
			c, _, _ = door.Pos.MoveBy(DirEast, 1)
			c, _, _ = c.MoveBy(DirSouth, 2)
			secondTileOffs = 0x01
		case DirSouth:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			c, _, _ = c.MoveBy(DirEast, 1)
			secondTileOffs = 0x01
		case DirEast:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			secondTileOffs = 0x40
		case DirWest:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			c, _, _ = c.MoveBy(DirEast, 2)
			secondTileOffs = 0x40
		}
		doorwayC := c

		count := 3
		v := tiles[doorwayC]

		dbg := strings.Builder{}
		if (v >= 0xF0 && v <= 0xF7) || (v >= 0xF8 && v <= 0xFF) {
			// find opposite end of matched doorway:
			for i := 0; i < 16; i++ {
				c, _, ok = c.MoveBy(door.Dir, 1)
				if !ok {
					break
				}
				fmt.Fprintf(&dbg, "%02X,", tiles[c])
				if count >= 4 {
					if !isTileCollision(tiles[c]) {
						count++
						break
					}
					if tiles[c] == v^8 {
						count++
						break
					}
				}
				count++
			}
		} else {
			// find how far to clear to opposite doorway:
			c = door.Pos
			for i := 0; i < 16; i++ {
				c, _, ok = c.MoveBy(door.Dir, 1)
				if !ok {
					break
				}
				fmt.Fprintf(&dbg, "%02X,", tiles[c])
				if tiles[c] == 0x02 && count >= 8 {
					count++
					break
				}
				count++
			}
		}

		fmt.Printf("$%03X: door type=%s dir=%s pos=%s: %s\n", uint16(t.Supertile), door.Type, door.Dir, door.Pos, dbg.String())

		c, ok = doorwayC, true

		// doors only allow bidirectional travel:
		allowDirFlags := uint8(1<<uint8(door.Dir)) | uint8(1<<uint8(door.Dir.Opposite()))
		if tiles[c] == 0x8E || tiles[c] == 0x8F || door.Type == 0x2A {
			// exit/entrance doorways cannot allow exit traversal:
			allowDirFlags = uint8(1 << uint8(door.Dir.Opposite()))
		}

		// clear the path:
		for i := 0; ok && i < count; i++ {
			room.AllowDirFlags[c] = allowDirFlags
			room.AllowDirFlags[c+secondTileOffs] = allowDirFlags

			// only replace tiles that are collision types:
			if isTileCollision(tiles[c]) {
				tiles[c] = 0
			}
			if isTileCollision(tiles[c+secondTileOffs]) {
				tiles[c+secondTileOffs] = 0
			}

			c, _, ok = c.MoveBy(door.Dir, 1)
		}
	}

	copy(room.WRAMAfterLoaded[:], (*e.WRAM)[:])

	// persist the current TilesVisited map in its hash(tiles) slot:
	room.SwapTilesVisitedMap()

	// os.WriteFile(fmt.Sprintf("r%03X.post.tmap", uint16(t.Supertile)), tiles, 0644)
	// os.WriteFile(fmt.Sprintf("r%03X.dir.tmap", uint16(t.Supertile)), room.AllowDirFlags[:], 0644)

	return
}

func tileAllowableDirFlags(v uint8) uint8 {
	// north/south doorways:
	if v == 0x80 || v == 0x82 || v == 0x84 || v == 0x86 || v == 0x8E || v == 0x8F || v == 0xA0 ||
		v == 0x5E || v == 0x5F || v&0xF8 == 0x30 || v == 0x38 || v == 0x39 || v == 0x28 || v == 0x29 {
		return 0b0000_0011
	}
	// east/west doorways:
	if v == 0x81 || v == 0x83 || v == 0x85 || v == 0x87 || v == 0x89 || v == 0x2A || v == 0x2B {
		return 0b0000_1100
	}

	// somaria:
	if v == 0xB0 {
		return 0b0000_1100
	} else if v == 0xB1 {
		return 0b0000_0011
	} else if v == 0xB2 {
		return 0b0000_1010
	} else if v == 0xB3 {
		return 0b0000_1001
	} else if v == 0xB4 {
		return 0b0000_0110
	} else if v == 0xB5 {
		return 0b0000_0101
	} else if v == 0xB6 {
		return 0b0000_1111
	} else if v == 0xB7 {
		return 0b0000_1110
	} else if v == 0xB8 {
		return 0b0000_1101
	} else if v == 0xB9 {
		return 0b0000_1011
	} else if v == 0xBA {
		return 0b0000_0111
	} else if v == 0xBB {
		return 0b0000_1111
	} else if v == 0xBC {
		return 0b0000_1111
	} else if v == 0xBD {
		return 0b0000_1111
	} else if v == 0xBE {
		return 0b0000_1111
	}

	// collision prevents traversal:
	if isTileCollision(v) {
		return 0
	}

	// otherwise allow all 4 directions:
	return 0b0000_1111
}

func isTileCollision(v uint8) bool {
	// entrance doors:
	if v == 0x8E || v == 0x8F {
		return false
	}
	// spiral staircases:
	if v == 0x5E || v == 0x5F {
		return false
	}

	// moving floor:
	if v == 0x1C || v == 0x0C {
		return false
	}

	isWalkable := v == 0x00 || // no collision
		v == 0x09 || // shallow water
		v == 0x22 || // manual stairs
		v == 0x23 || v == 0x24 || // floor switches
		(v >= 0x0D && v <= 0x0F) || // spikes / floor ice
		v == 0x3A || v == 0x3B || // star tiles
		v == 0x40 || // thick grass
		v == 0x4B || // warp
		v == 0x60 || // rupee tile
		(v >= 0x68 && v <= 0x6B) || // conveyors
		v == 0xA0 // north/south dungeon swap door (for HC to sewers)

	if isWalkable {
		return false
	}

	vClass := v & 0xF0

	isMaybeWalkable := vClass == 0x70 || // pots/pegs/blocks
		v == 0x62 || // bombable floor
		v == 0x66 || v == 0x67 // crystal pegs (orange/blue):

	if isMaybeWalkable {
		return false
	}

	if vClass == 0x30 || vClass == 0x80 || vClass == 0x90 ||
		vClass == 0xA0 || vClass == 0xF0 {
		return false
	}

	return true
}

func reachTaskFloodfill(q Q, t T, room *RoomState) {
	// don't have two or more goroutines clobbering the same room:
	// NOTE: this causes deadlock if the job queue channel size is too small
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	st := t.Supertile

	wram := room.WRAM[:]
	tiles := wram[0x12000:0x14000]

	pushJob := func(neighborSt Supertile, se SE) {
		q.SubmitJob(
			&ReachTask{
				InitialEmulator: t.InitialEmulator,
				EntranceWRAM:    t.EntranceWRAM,
				EntranceVRAM:    t.EntranceVRAM,
				EntranceID:      t.EntranceID,
				Rooms:           t.Rooms,
				RoomsLock:       t.RoomsLock,
				Supertile:       neighborSt,
				SE:              se,
			},
			ReachTaskInterRoom,
		)
	}

	startStates := make([]SE, 0, 32)

	// push kill-all-enemies state:
	startStates = append(startStates, SE{
		c:    t.SE.c,
		d:    t.SE.d,
		s:    6,
		wram: nil,
	})

	// push starting state:
	startStates = append(startStates, t.SE)

	lifo := make([]SE, 0, 1024)

	for len(startStates) > 0 {
		startState := startStates[len(startStates)-1]
		startStates = startStates[:len(startStates)-1]

		fmt.Printf("$%03X: start=%04X dir=%5s state=%d\n", uint16(t.Supertile), uint16(startState.c), startState.d, startState.s)

		if startState.s == 4 || startState.s == 5 || startState.s == 6 {
			// process tags:
			// restore WRAM before processing tags:
			if startState.wram != nil {
				copy(wram[:], (*startState.wram)[:])
			}
			if startState.s == 5 {
				// manipulable push block always sets this when pushed:
				write8(wram, 0x0641, 0x01)
			} else if startState.s == 6 {
				// kill all enemies:
				killedOne := false
				for j := uint32(0); j < 16; j++ {
					if read8(wram, 0x0F60+j)&0x40 != 0 {
						// ignore for kill rooms:
						continue
					}
					if read8(wram, 0x0DD0+j) != 0 {
						write8(wram, 0x0DD0+j, 0)
						killedOne = true
					}
				}
				if killedOne {
					fmt.Printf("$%03X: kill room\n", uint16(t.Supertile))
				} else {
					// nothing killed so no tag to process:
					continue
				}
			}
			// os.WriteFile(fmt.Sprintf("r%03X.%08X.tmap", uint16(t.Supertile), room.CalcTilesHash()), tiles, 0644)
			// fmt.Printf("$%03X: tags: run; old [04BC]=%02X\n", uint16(t.Supertile), wram[0x04BC])
			room.PlaceLinkAt(startState.c)
			room.ProcessRoomTags()
			room.RecalcAllowDirFlags()
			room.SwapTilesVisitedMap()
			// {
			// 	tmp := [0x2000]byte{}
			// 	for i := range room.TilesVisited {
			// 		tmp[i] = 0x40
			// 	}
			// 	os.WriteFile(fmt.Sprintf("r%03X.%08X.visited.tmap", uint16(t.Supertile), room.CalcTilesHash()), tmp[:], 0644)
			// }
			// fmt.Printf("$%03X: tags: ran; new [04BC]=%02X\n", uint16(t.Supertile), wram[0x04BC])

			startState.s = 0
		} else {
			// start from the initial load state:
			copy(room.WRAM[:], room.WRAMAfterLoaded[:])
			room.SwapTilesVisitedMap()
		}

		lifo = append(lifo, startState)

		// iteratively recurse over processing stack:
		for len(lifo) > 0 {
			se := lifo[len(lifo)-1]
			lifo = lifo[0 : len(lifo)-1]

			c := se.c
			v := tiles[c]

			if _, ok := room.TilesVisited[se.c]; ok {
				continue
			}
			room.TilesVisited[se.c] = empty{}

			layerSwap := MapCoord(0)

			// default traversal state:
			canTraverse := false
			canTurn := true
			traverseDir := se.d
			traverseBy := 1
			// fmt.Printf("$%03X: [$%04X]=%02X\n", uint16(st), uint16(se.c), v)

			if se.s == 2 {
				// somaria:
				if v == 0xB6 {
					// end somaria (parallel):
					canTurn = true
					if ct, d, ok := c.MoveBy(traverseDir, 3); ok && !isTileCollision(tiles[ct]) {
						lifo = append(lifo, SE{c: ct, d: d, s: 0})
					}
				} else if v == 0xBC {
					// end somaria (perpendicular):
					canTurn = true
					if ct, d, ok := c.MoveBy(traverseDir.RotateCW(), 3); ok && !isTileCollision(tiles[ct]) {
						lifo = append(lifo, SE{c: ct, d: d, s: 0})
					}
					if ct, d, ok := c.MoveBy(traverseDir.RotateCCW(), 3); ok && !isTileCollision(tiles[ct]) {
						lifo = append(lifo, SE{c: ct, d: d, s: 0})
					}
				} else if v == 0xBD {
					// somaria cross-over:
					canTurn = false
					delete(room.TilesVisited, c)
				} else if v >= 0xB0 && v <= 0xBD {
					canTurn = true
				} else if v == 0xBE {
					// pipe exit:
					canTurn = false
					se.s = 0
				}

				room.Reachable[c] = v
				if canTurn {
					// turn from here:
					if c, d, ok := room.AttemptTraversal(c, traverseDir.RotateCW(), traverseBy); ok {
						lifo = append(lifo, SE{c: c, d: d, s: se.s})
					}
					if c, d, ok := room.AttemptTraversal(c, traverseDir.RotateCCW(), traverseBy); ok {
						lifo = append(lifo, SE{c: c, d: d, s: se.s})
					}
				}
				// traverse in the primary direction:
				if c, d, ok := room.AttemptTraversal(c, traverseDir, traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
				continue
			} else if se.s == 3 {
				// pipes:
				allowDirFlags := byte(1 << traverseDir)
				if v == 0xBE {
					// pipe exit:
					se.s = 0
				} else if v == 0xB2 {
					// north/east turn:
					if traverseDir == DirNorth {
						traverseDir = DirEast
					} else if traverseDir == DirWest {
						traverseDir = DirSouth
					}
					allowDirFlags = 1 << traverseDir
				} else if v == 0xB3 {
					// south/east turn:
					if traverseDir == DirSouth {
						traverseDir = DirEast
					} else if traverseDir == DirWest {
						traverseDir = DirNorth
					}
					allowDirFlags = 1 << traverseDir
				} else if v == 0xB4 {
					// north/west turn:
					if traverseDir == DirNorth {
						traverseDir = DirWest
					} else if traverseDir == DirEast {
						traverseDir = DirSouth
					}
					allowDirFlags = 1 << traverseDir
				} else if v == 0xB5 {
					// east/north turn:
					if traverseDir == DirEast {
						traverseDir = DirNorth
					} else if traverseDir == DirSouth {
						traverseDir = DirWest
					}
					allowDirFlags = 1 << traverseDir
				} else if v == 0xBD {
					// pipe cross-over:
					// allow cross-over to be revisited:
					delete(room.TilesVisited, c)
				} else {
					// allow collision tiles to be revisited in case of cross-over:
					delete(room.TilesVisited, c)
				}

				room.Reachable[c] = v
				// traverse in the primary direction:
				if c, d, ok := attemptTraversal(c, allowDirFlags, traverseDir, traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
				continue
			} else if se.s == 7 {
				// swimming:
				if v == 0x08 {
					canTraverse = true
					canTurn = true
				} else if v == 0x0A || v == 0x1D || v == 0x3D {
					// 0A - deep water ladder
					// 1D - north stairs
					// 3D - south stairs
					ct := c & ^MapCoord(0x1000)
					d := traverseDir
					if v == 0x1D {
						d = DirNorth
					} else if v == 0x3D {
						d = DirSouth
					}
					if ct, _, ok := ct.MoveBy(d, 1); ok {
						lifo = append(lifo, SE{c: ct, d: d, s: 0})
					}
					continue
				}

				if !canTraverse {
					continue
				}

				room.Reachable[c] = v

				if canTurn {
					// turn from here:
					if c, d, ok := c.MoveBy(traverseDir.Opposite(), traverseBy); ok {
						lifo = append(lifo, SE{c: c, d: d, s: se.s})
					}
					if c, d, ok := c.MoveBy(traverseDir.RotateCW(), traverseBy); ok {
						lifo = append(lifo, SE{c: c, d: d, s: se.s})
					}
					if c, d, ok := c.MoveBy(traverseDir.RotateCCW(), traverseBy); ok {
						lifo = append(lifo, SE{c: c, d: d, s: se.s})
					}
				}
				// traverse in the primary direction:
				if c, d, ok := c.MoveBy(traverseDir, traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
				continue
			}

			if v >= 0x80 && v <= 0x8D {
				// traveling through a doorway:
				// TODO: special case for 0x89 transport door
				initialV := v
				canTraverse = true
				canTurn = false
				// don't advance beyond the end of the doorway:
				traverseBy = 0

				fmt.Printf("$%03X: doorway $%04X %s\n", uint16(t.Supertile), uint16(c), se.d)

				// try to find a layer-swap tile in the doorway:
				ok := true
				for i := 0; ok && i < 16; i++ {
					v = tiles[c]
					if v&0xF0 == 0x90 || v&0xF8 == 0xA8 {
						// only swap layers if we're traversing the doorway initially, there may be a layer swap on the opposite side:
						if se.s == 0 {
							layerSwap = 0x1000
							se.s = 1
						}
					} else if v != initialV {
						// stop when we're out of the doorway tiles
						fmt.Printf("$%03X: doorway stop $%04X %s\n", uint16(t.Supertile), uint16(c), se.d)
						se.s = 0
						break
					}

					room.Reachable[c] = v
					room.TilesVisited[c] = empty{}

					// or stop if we hit the edge:
					c, _, ok = c.MoveBy(se.d, 1)
				}

				if ok, _, _, _ = c.IsEdge(); ok {
					// hit the edge:
					if initialV == 0x89 {
						// east/west transport door needs a special exit supertile, not its neighbor
						neighborSt := room.StairExitTo[traverseDir]
						fmt.Printf("$%03X: edge $%04X %s teleport exit to $%03X\n", uint16(t.Supertile), uint16(c), traverseDir, uint16(neighborSt))
						pushJob(
							neighborSt,
							SE{
								c: c.OppositeEdge() ^ layerSwap,
								d: traverseDir,
								s: se.s,
							})
						continue
					}
				} else {
					// if we did not hit the edge then just do the layer swap if applicable:
					c ^= layerSwap
					layerSwap = 0
					se.s = 0
				}
			} else if v == 0x1D || v == 0x3D {
				// north or south single-layer auto-stairs:
				initialV := v
				ok := true
				for i := 0; ok && i < 2; i++ {
					v = tiles[c]
					if v != initialV {
						break
					}
					room.Reachable[c] = v
					room.TilesVisited[c] = empty{}
					c, _, ok = c.MoveBy(se.d, 1)
				}
				if ok {
					canTraverse = true
					canTurn = false
				}
			} else if v == 0x1E || v == 0x1F || v == 0x3E || v == 0x3F {
				// north or south layer-toggle auto-stairs:
				initialV := v
				ok := true
				for i := 0; ok && i < 2; i++ {
					v = tiles[c]
					if v != initialV {
						break
					}
					room.Reachable[c] = v
					room.TilesVisited[c] = empty{}
					c, _, ok = c.MoveBy(se.d, 1)
				}
				if ok {
					canTraverse = true
					canTurn = false
					c ^= 0x1000
				}
			} else if v == 0x5E || v == 0x5F || v&0xF8 == 0x30 || v&0xF0 == 0xF0 {
				// doors may cover in front of stairs
				room.Reachable[c] = v

				// find the stair tile:
				var stairExit byte
				var stairKind byte
				for i := 0; i < 4; i++ {
					cs, _, _ := c.MoveBy(traverseDir, i)
					v = tiles[cs]
					room.Reachable[cs] = v
					if v >= 0x30 && v <= 0x37 {
						stairExit = v
						if stairKind == 0 {
							stairKind = v
						}
					} else if v >= 0x38 && v <= 0x39 {
						stairKind = v
					} else if v == 0x5E || v == 0x5F {
						stairKind = v
					} else if v&0xF0 == 0xF0 {
						continue
					} else {
						break
					}
				}

				if stairKind == 0 {
					canTraverse = true
					canTurn = false
				} else {
					// move south of the stair tile:
					ct, _, _ := c.MoveBy(traverseDir.Opposite(), 1)

					fmt.Printf(
						"$%03X: debug stairs at %04X exit=%02X, kind=%02X\n",
						uint16(t.Supertile),
						uint16(c),
						stairExit,
						stairKind,
					)

					var neighborSt Supertile
					if stairKind == 0x38 {
						// north stairs:
						neighborSt = room.StairExitTo[stairExit&3]
						traverseDir = DirNorth

						// set destination layer:
						ct = ct&0x0FFF | room.StairTargetLayer[stairExit&3]
						ct += 0x0D40

						// adjust destination based on layer swap:
						if stairExit&0x04 == 0 {
							// going up
							if c&0x1000 != 0 {
								ct -= 0xC0
							}
							if ct&0x1000 != 0 {
								ct -= 0xC0
							}
						} else {
							// going down
							if c&0x1000 != 0 {
								ct += 0xC0
							}
							if ct&0x1000 != 0 {
								ct += 0xC0
							}
						}
					} else if stairKind == 0x39 {
						// south stairs:
						neighborSt = room.StairExitTo[stairExit&3]
						traverseDir = DirSouth

						// set destination layer:
						ct = ct&0x0FFF | room.StairTargetLayer[stairExit&3]
						ct -= 0x0D40

						// adjust destination based on layer swap:
						if stairExit&0x04 == 0 {
							// going up
							if c&0x1000 != 0 {
								ct -= 0xC0
							}
							if ct&0x1000 != 0 {
								ct -= 0xC0
							}
						} else {
							// going down
							if c&0x1000 != 0 {
								ct += 0xC0
							}
							if ct&0x1000 != 0 {
								ct += 0xC0
							}
						}
					} else if stairKind == 0x5E || stairKind == 0x5F {
						// inter-room stairs:
						neighborSt = room.StairExitTo[stairExit&3]
						traverseDir = DirSouth

						// set destination layer:
						ct = ct&0x0FFF | room.StairTargetLayer[stairExit&3]

						// adjust destination based on layer swap:
						if stairExit&0x04 == 0 {
							// going up
							if c.IsLayer2() && !ct.IsLayer2() {
								ct -= 0xC0
							} else if !c.IsLayer2() && ct.IsLayer2() {
								ct += 0xC0
							}
						} else {
							// going down
							if c.IsLayer2() && !ct.IsLayer2() {
								ct -= 0xC0
							} else if !c.IsLayer2() && ct.IsLayer2() {
								ct += 0xC0
							}
						}
					} else if stairKind >= 0x30 && stairKind <= 0x37 {
						// straight inter-room stairs:
						neighborSt = room.StairExitTo[stairExit&3]
						ct, _, _ = c.MoveBy(traverseDir, 1)

						// set destination layer:
						ct = ct&0x0FFF | room.StairTargetLayer[stairExit&3]
					}

					fmt.Printf("$%03X: stairs $%04X exit to $%03X at $%04X\n", uint16(t.Supertile), uint16(c), uint16(neighborSt), uint16(ct))
					pushJob(
						neighborSt,
						SE{
							c: ct,
							d: traverseDir,
							s: se.s,
						})
					continue
				}
			} else if v == 0xBE {
				// pipe entry:
				canTraverse = true
				canTurn = false
				se.s = 3
			} else if v == 0x28 {
				// 28 - North ledge
				canTurn = false
				if traverseDir == DirNorth || traverseDir == DirSouth {
					canTraverse = true
					// traverseDir = DirNorth
					traverseBy = 5
				}
			} else if v == 0x29 {
				// 29 - South ledge
				canTurn = false
				if traverseDir == DirNorth || traverseDir == DirSouth {
					canTraverse = true
					// traverseDir = DirSouth
					traverseBy = 5
				}
			} else if v == 0x2A {
				// 2A - East ledge
				canTurn = false
				if traverseDir == DirWest || traverseDir == DirEast {
					canTraverse = true
					// traverseDir = DirEast
					traverseBy = 5
				}
			} else if v == 0x2B {
				// 2B - West ledge
				canTurn = false
				if traverseDir == DirWest || traverseDir == DirEast {
					canTraverse = true
					// traverseDir = DirWest
					traverseBy = 5
				}
			} else if v == 0x08 {
				// 08 - deep water
				// room 76 has this on layer 1 in place of a normal stairwell into the pool
				if ct, d, ok := c.MoveBy(traverseDir, 2); ok {
					lifo = append(lifo, SE{c: ct, d: d, s: 0})
				}
			} else if v == 0x1C && !c.IsLayer2() {
				if tiles[c|0x1000] == 0x0C {
					canTraverse = true
					canTurn = true
				} else {
					// transition from layer 1 to layer 2:
					fmt.Printf("$%03X: fall $%04X\n", uint16(t.Supertile), uint16(c))
					lifo = append(lifo, SE{c: c | 0x1000, d: traverseDir, s: se.s})
				}
			} else if v == 0x20 || v == 0x62 {
				// pit or bombable floor:
				room.Reachable[c] = v
				room.HasReachablePit = true
				if !roomsWithPitDamage[st] {
					// exit via pit:
					neighborSt := room.WarpExitTo
					ct := c&0x0FFF | room.WarpExitLayer
					fmt.Printf("$%03X: pit $%04X %s exit to $%03X at %04X\n", uint16(t.Supertile), uint16(c), traverseDir, uint16(neighborSt), uint16(ct))
					pushJob(
						neighborSt,
						SE{
							c: ct,
							d: traverseDir,
							s: se.s,
						})
				}
				if ct, _, ok := c.MoveBy(traverseDir, 2); ok && (tiles[ct] == 0xB6 || tiles[ct] == 0xBC) {
					// start somaria from across a pit:
					lifo = append(lifo, SE{c: ct, d: traverseDir, s: 2})
				}
			} else if v&0xF0 == 0x80 {
				// shutter doors and entrance doors
				canTraverse = true
				canTurn = false
			} else if v&0xF0 == 0xF0 {
				// doorways:
				canTraverse = true
				canTurn = false
			} else if v&0xF0 == 0x70 {
				// manipulables:
				canTraverse = true
				canTurn = true
			} else if room.isAlwaysWalkable(v) || room.isMaybeWalkable(c, v) {
				canTraverse = true

				// check for water below us:
				{
					ct := c | 0x1000
					if ct != c && tiles[ct] == 0x08 {
						// if v != 0x08 && v != 0x0D {
						// 	r.pushAllDirections(t, StateSwim)
						// }
						// start swimming:
						lifo = append(lifo, SE{c: ct, d: traverseDir, s: 7})
					}
				}

				// can we hookshot to something?
				for d := DirNorth; d < DirNone; d++ {
					canHook := false
					requiresHook := false
					ct, ok := c, true
					hookOverTiles := make([]MapCoord, 0, 16)
					mode := 0
					ledges := 0
					for i := 1; i <= 16; i++ {
						if ct, _, ok = ct.MoveBy(d, 1); !ok {
							canHook = false
							break
						}

						if mode == 0 {
							// we must pass over something impassable to make the hookshot necessary:
							if tiles[ct] == 0x28 || tiles[ct] == 0x29 {
								requiresHook = true
								ledges ^= 1
								mode = 1
								continue
							} else if tiles[ct] == 0x2A || tiles[ct] == 0x2B {
								requiresHook = true
								ledges ^= 1
								mode = 2
								continue
							}

							if tiles[ct] == 0x20 || tiles[ct] == 0x1C {
								requiresHook = true
							}

							if room.canHookThru(tiles[ct]) {
								hookOverTiles = append(hookOverTiles, ct)
								continue
							} else if room.isHookable(tiles[ct]) {
								// stop the hook if we hit a wall or something:
								canHook = true
								break
							}

							// otherwise we can't hook through or to this:
							canHook = false
							break
						} else if mode == 1 {
							// north/south ledges:
							if tiles[ct] == 0x28 || tiles[ct] == 0x29 {
								requiresHook = true
								ledges ^= 1
								mode = 0
							}
						} else if mode == 2 {
							if tiles[ct] == 0x2A || tiles[ct] == 0x2B {
								requiresHook = true
								ledges ^= 1
								mode = 0
							}
						}
					}

					// must have even number of ledge pairs:
					if canHook && requiresHook && ledges == 0 {
						// prove we have space to land at:
						ct, _, _ = ct.MoveBy(d.Opposite(), 2)
						if room.isAlwaysWalkable(tiles[ct]) {
							if _, ok := room.TilesVisited[ct]; !ok {
								fmt.Printf("$%03X: hook %s at $%04X to %04X\n", uint16(t.Supertile), d, uint16(c), uint16(ct))
								lifo = append(lifo, SE{c: ct, d: d, s: se.s})

								// mark tiles as hookable:
								for _, ch := range hookOverTiles {
									room.Hookshot[ch] |= byte(1 << d)
								}
							}
						}
					}
				}
			}

			// star tiles ONLY trigger when x,y is at the top-left of the 2x2 tile.
			v2x2 := uint32(read16(tiles, uint32(c)))<<16 | uint32(read16(tiles, uint32(c+0x40)))
			if v2x2 == 0x3A3A3A3A || v2x2 == 0x3B3B3B3B {
				// 3A - inactive star tile
				// 3B - active star tile
				canTraverse = true
				canTurn = true
				fmt.Printf("$%03X: star at $%04X\n", uint16(t.Supertile), uint16(c))
				// make a WRAM copy to resume from:
				wramCopy := new(WRAMArray)
				copy((*wramCopy)[:], room.WRAM[:])
				// process star tile switch after current floodfill exhausts itself with the current room state:
				startStates = append(startStates, SE{c: c, d: traverseDir, s: 4, wram: wramCopy})
			} else if v2x2 == 0x4B4B4B4B {
				// 4B - warp tile
				neighborSt := room.WarpExitTo
				ct := c | room.WarpExitLayer
				fmt.Printf("$%03X: warp $%04X exit to $%03X at %04X\n", uint16(t.Supertile), uint16(c), uint16(neighborSt), uint16(ct))
				pushJob(
					neighborSt,
					SE{
						c: ct,
						d: traverseDir,
						s: se.s,
					})
				canTraverse = true
			} else if v2x2&0xF0F0F0F0 == 0x70707070 {
				// manipulable block:
				j := v & 0x0F
				manipProps := read16(wram, 0x0500+uint32(j)<<1)
				if manipProps == 0x0000 {
					// push block:
					// make a WRAM copy to resume from:
					wramCopy := new(WRAMArray)
					copy((*wramCopy)[:], room.WRAM[:])
					// process block manipulation after current floodfill exhausts itself with the current room state:
					startStates = append(startStates, SE{c: c, d: traverseDir, s: 5, wram: wramCopy})
				}
			}

			// can we bonk cross a pit from this bonkable tile?
			if v == 0x27 || v&0xF0 == 0x70 {
				for d := DirNorth; d < DirNone; d++ {
					// need a place to bonk from:
					canBonk := true
					ct, _, ok := c.MoveBy(d, 1)
					if !ok {
						canBonk = false
					}
					if !room.isAlwaysWalkable(tiles[ct]) {
						canBonk = false
					}
					ct, _, ok = c.MoveBy(d, 2)
					if !ok {
						canBonk = false
					}
					if !room.isAlwaysWalkable(tiles[ct]) {
						canBonk = false
					}

					if canBonk {
						// prove all tiles in between are pit:
						hasPit := false
						ct = c
						for i := 1; i <= 9; i++ {
							if ct, _, ok = ct.MoveBy(d, 1); !ok {
								canBonk = false
								break
							}
							if tiles[ct] == 0x20 {
								hasPit = true
								continue
							} else if room.isAlwaysWalkable(tiles[ct]) {
								continue
							} else if isTileCollision(tiles[ct]) {
								// stop the bonk if we hit a wall or something:
								break
							}
							canBonk = false
							break
						}
						if canBonk && hasPit {
							fmt.Printf("$%03X: pit bonk skip %s at $%04X off %02X\n", uint16(t.Supertile), d, uint16(c), v)
							lifo = append(lifo, SE{c: ct, d: d, s: se.s})
						}
					}
				}
			}

			if !canTraverse {
				continue
			}

			room.Reachable[c] = v

			// transition to neighboring room at the edges:
			if ok, edgeDir, _, _ := c.IsEdge(); ok {
				if traverseDir == edgeDir && room.CanTraverseDir(c, traverseDir) {
					if neighborSt, _, ok := st.MoveBy(traverseDir); ok {
						fmt.Printf("$%03X: edge $%04X %s exit to $%03X\n", uint16(t.Supertile), uint16(c), traverseDir, uint16(neighborSt))
						pushJob(
							neighborSt,
							SE{
								c: c.OppositeEdge() ^ layerSwap,
								d: traverseDir,
								s: se.s,
							})
						continue
					}
				}
			}

			if canTurn {
				// turn from here:
				if c, d, ok := room.AttemptTraversal(c, traverseDir.Opposite(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
				if c, d, ok := room.AttemptTraversal(c, traverseDir.RotateCW(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
				if c, d, ok := room.AttemptTraversal(c, traverseDir.RotateCCW(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: se.s})
				}
			}
			// traverse in the primary direction:
			if c, d, ok := room.AttemptTraversal(c, traverseDir, traverseBy); ok {
				lifo = append(lifo, SE{c: c, d: d, s: se.s})
			}
		}
	}
}

func canTraverseDir(allowDirFlags byte, d Direction) bool {
	return allowDirFlags&uint8(1<<d) != 0
}

func attemptTraversal(c MapCoord, allowDirFlags byte, d Direction, by int) (nc MapCoord, nd Direction, ok bool) {
	if by == 0 {
		nc, nd, ok = c, d, true
		return
	}

	if !canTraverseDir(allowDirFlags, d) {
		nc, nd, ok = c, d, false
		return
	}

	nc, nd, ok = c.MoveBy(d, by)
	return
}

func (room *RoomState) CanTraverseDir(c MapCoord, d Direction) bool {
	f := room.AllowDirFlags[c]
	return canTraverseDir(f, d)
}

func (room *RoomState) AttemptTraversal(c MapCoord, d Direction, by int) (nc MapCoord, nd Direction, ok bool) {
	if by == 0 {
		nc, nd, ok = c, d, true
		return
	}

	if !room.CanTraverseDir(c, d) {
		nc, nd, ok = c, d, false
		return
	}

	nc, nd, ok = c.MoveBy(d, by)
	return
}

func (room *RoomState) PlaceLinkAt(c MapCoord) {
	wram := room.WRAM[:]

	// set absolute x,y coordinates to the tile:
	x, y := c.ToAbsCoord(room.Supertile)
	write16(wram, 0x20, y)
	write16(wram, 0x22, x)
	write16(wram, 0xEE, (uint16(c)&0x1000)>>12)

	// ensure link on screen:
	write16(wram, 0x00E2, uint16(int(x)-0x40))
	write16(wram, 0x00E8, uint16(int(y)-0x40))
}

func (r *RoomState) ProcessRoomTags() {
	e := r.e
	if e.WRAM != &r.WRAM {
		panic("NOPE")
	}
	wram := (r.WRAM)[:]

	// if no tags present, don't check them:
	oldAE, oldAF := read8(wram, 0xAE), read8(wram, 0xAF)
	if oldAE == 0 && oldAF == 0 {
		fmt.Printf("$%03X: tags: no tags to activate\n", uint16(r.Supertile))
		return
	}

	// old04BC := read8(wram, 0x04BC)

	// e.CPU.OnWDM = func(wdm byte) {
	// 	// capture frame to GIF:
	// 	if wdm == 0xFF {
	// 		fmt.Println("WDM: frame")
	// 	}
	// }

	// e.LoggerCPU = os.Stdout
	if err := e.ExecAt(b00HandleRoomTagsPC, 0); err != nil {
		panic(err)
	}
	// e.LoggerCPU = nil

	// e.CPU.OnWDM = nil

	return
}

func (room *RoomState) RecalcAllowDirFlags() {
	tilesOld := room.WRAMAfterLoaded[0x12000:0x14000]
	tiles := room.WRAM[0x12000:0x14000]

	for i := range room.AllowDirFlags {
		// don't update AllowDirFlags if tiles haven't changed:
		if tilesOld[i] == tiles[i] {
			continue
		}

		room.AllowDirFlags[i] = tileAllowableDirFlags(tiles[i])
	}
}

func (room *RoomState) CalcTilesHash() (tilesHash uint64) {
	h := fnv.New64()
	h.Write(room.WRAM[0x12000:0x14000])
	tilesHash = h.Sum64()

	return
}

func (room *RoomState) SwapTilesVisitedMap() {
	tilesHash := room.CalcTilesHash()
	// fmt.Printf("$%03X: tiles hash=%08X\n", uint16(room.Supertile), tilesHash)
	// os.WriteFile(fmt.Sprintf("r%03X.%08X.tmap", uint16(room.Supertile), tilesHash), room.WRAM[0x12000:0x14000], 0644)

	if m, ok := room.TilesVisitedHash[tilesHash]; ok {
		room.TilesVisited = m
		return
	}

	room.TilesVisited = make(map[MapCoord]struct{}, 0x2000)
	room.TilesVisitedHash[tilesHash] = room.TilesVisited

	// render the new room state iff it's not been seen yet:
	if false {
		g := image.NewNRGBA(image.Rect(0, 0, 512, 512))
		room.renderToNonPaletted(g)
		exportPNG(fmt.Sprintf("r%03X.%08X.png", uint16(room.Supertile), tilesHash), g)
	}
}

func (room *RoomState) RenderToNonPaletted() {
	g := image.NewNRGBA(image.Rect(0, 0, 512, 512))

	room.renderToNonPaletted(g)

	// store full underworld rendering for inclusion into EG map:
	room.Rendered = g
	room.RenderedNRGBA = g
}

func (room *RoomState) renderToNonPaletted(g *image.NRGBA) {
	// render BG layers:
	// e.VRAM[0x4000:0x8000]
	pal, bg1p, bg2p, addColor, halfColor := renderBGLayers(&room.WRAM, room.VRAMTileSet[:])

	ComposeToNonPaletted(g, pal, bg1p, bg2p, addColor, halfColor)

	// overlay doors in blue rectangles:
	clrBlue := image.NewUniform(color.NRGBA{0, 0, 255, 192})
	for _, door := range room.Doors {
		drawShadowedString(
			g,
			image.White,
			fixed.Point26_6{X: fixed.I(int(door.Pos.Col()*8) + 8), Y: fixed.I(int(door.Pos.Row()*8) - 2)},
			fmt.Sprintf("%02X", uint8(door.Type)),
		)
		drawOutlineBox(
			g,
			image.NewUniform(clrBlue),
			int(door.Pos.Col()*8),
			int(door.Pos.Row()*8),
			4*8,
			4*8,
		)
		drawOutlineBox(
			g,
			image.NewUniform(clrBlue),
			int(door.Pos.Col()*8)-1,
			int(door.Pos.Row()*8)-1,
			4*8+2,
			4*8+2,
		)
	}

	// tags:
	drawShadowedString(
		g,
		image.White,
		fixed.Point26_6{X: fixed.I(32), Y: fixed.I(16)},
		fmt.Sprintf("%02X %02X", uint8(room.WRAM[0xAE]), uint8(room.WRAM[0xAF])),
	)
}
