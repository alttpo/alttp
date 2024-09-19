package main

import (
	"fmt"
	"image"
	"image/gif"
	"os"
	"roomloader/taskqueue"
	"strings"
	"sync"
)

type ReachTask struct {
	InitialEmulator *System

	Rooms     map[uint16]*RoomState
	RoomsLock *sync.Mutex

	EntranceID uint8
	Supertile  Supertile
	Start      MapCoord
	Direction  Direction
}

type T = *ReachTask
type Q = *taskqueue.Q[T]

func ReachTaskInterRoom(q Q, t T) {
	var err error

	e := &System{}
	if err = e.InitEmulatorFrom(t.InitialEmulator); err != nil {
		panic(err)
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

	reachTaskFloodfill(q, t, e)
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

	t.Start = linkC
	t.Direction = linkD

	room := reachTaskFloodfill(q, t, e)

	// mark Link's entry position:
	room.Reachable[t.Start] = 0xFF
}

func getOrCreateRoom(t T, e *System) (room *RoomState) {
	t.RoomsLock.Lock()
	defer t.RoomsLock.Unlock()

	var ok bool
	room, ok = t.Rooms[uint16(t.Supertile)]
	if ok {
		return
	}

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
		TilesVisited:     make(map[MapCoord]struct{}, 0x2000),
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
	t.Rooms[uint16(t.Supertile)] = room

	// do first-time room processing work:

	for i := range room.Reachable {
		room.Reachable[i] = 0x01
		// all 4 directions are allowable by default:
		room.AllowDirFlags[i] = 0b00001111
	}

	wram := (e.WRAM)[:]
	tiles := wram[0x12000:0x14000]

	os.WriteFile(fmt.Sprintf("r%03X.pre.tmap", uint16(t.Supertile)), tiles, 0644)

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
			}
			if tiles[t|0x1000] == 0x58+uint8(i) {
				tiles[t|0x1000+0x00] = 0x00
				tiles[t|0x1000+0x01] = 0x00
				tiles[t|0x1000+0x40] = 0x00
				tiles[t|0x1000+0x41] = 0x00
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
		if door.Type == 0x30 {
			// exploding wall:
			pos := int(door.Pos)
			for c := 0; c < 11; c++ {
				for r := 0; r < 12; r++ {
					tiles[pos+(r<<6)-c] = 0
					tiles[pos+(r<<6)+1+c] = 0
				}
			}
			continue
		}

		room.Doors = append(room.Doors, door)

		var ok bool
		var c MapCoord

		// find the first doorway tile:
		var secondTileOffs MapCoord
		switch door.Dir {
		case DirNorth:
			c, _, _ = door.Pos.MoveBy(DirEast, 1)
			c, _, ok = c.MoveBy(DirSouth, 2)
			secondTileOffs = 0x01
		case DirSouth:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			c, _, ok = c.MoveBy(DirEast, 1)
			secondTileOffs = 0x01
		case DirEast:
			c, _, ok = door.Pos.MoveBy(DirSouth, 1)
			secondTileOffs = 0x40
		case DirWest:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			c, _, ok = c.MoveBy(DirEast, 2)
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
				if tiles[c] == v^8 && count >= 4 {
					count++
					break
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

	os.WriteFile(fmt.Sprintf("r%03X.post.tmap", uint16(t.Supertile)), tiles, 0644)
	os.WriteFile(fmt.Sprintf("r%03X.dir.tmap", uint16(t.Supertile)), room.AllowDirFlags[:], 0644)

	return
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

	if vClass == 0x30 || vClass == 0x80 || vClass == 0xF0 {
		return false
	}

	return true
}

func reachTaskFloodfill(q Q, t T, e *System) (room *RoomState) {
	room = getOrCreateRoom(t, e)
	st := t.Supertile

	// don't have two or more goroutines clobbering the same room:
	// NOTE: this causes deadlock if the job queue channel size is too small
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	wram := (e.WRAM)[:]
	tiles := wram[0x12000:0x14000]

	type SE struct {
		c MapCoord
		d Direction
		s int
	}
	lifo := make([]SE, 0, 1024)

	// push starting state:
	lifo = append(lifo, SE{c: t.Start, d: t.Direction, s: 0})

	fmt.Printf("$%03X: start=%04X dir=%s\n", uint16(t.Supertile), uint16(t.Start), t.Direction)

	// iteratively recurse over processing stack:
	for len(lifo) > 0 {
		se := lifo[len(lifo)-1]
		lifo = lifo[0 : len(lifo)-1]

		if _, ok := room.TilesVisited[se.c]; ok {
			continue
		}
		room.TilesVisited[se.c] = empty{}

		v := tiles[se.c]

		canTraverse := false
		canTurn := true
		traverseDir := se.d
		traverseBy := 1
		// fmt.Printf("$%03X: [$%04X]=%02X\n", uint16(st), uint16(se.c), v)

		if v == 0x20 {
			// pit:
			room.Reachable[se.c] = v
			room.HasReachablePit = true
		} else if room.isAlwaysWalkable(v) || room.isMaybeWalkable(se.c, v) {
			canTraverse = true
		} else if v&0xF0 == 0x80 {
			// shutter doors and entrance doors
			canTraverse = true
			canTurn = false
		} else if v&0xF0 == 0xF0 {
			// doorways:
			canTraverse = true
			canTurn = false
		} else if v == 0x28 {
			// 28 - North ledge
			canTraverse = true
			canTurn = false
			traverseDir = DirNorth
			traverseBy = 5
		} else if v == 0x29 {
			// 29 - South ledge
			canTraverse = true
			canTurn = false
			traverseDir = DirSouth
			traverseBy = 5
		} else if v == 0x2A {
			// 2A - East ledge
			canTraverse = true
			canTurn = false
			traverseDir = DirEast
			traverseBy = 5
		} else if v == 0x2B {
			// 2B - West ledge
			canTraverse = true
			canTurn = false
			traverseDir = DirWest
			traverseBy = 5
		}

		if canTraverse {
			room.Reachable[se.c] = v

			// transition to neighboring room at the edges:
			if ok, edgeDir, _, _ := se.c.IsEdge(); ok {
				if traverseDir == edgeDir && room.CanTraverseDir(se.c, traverseDir) {
					if neighborSt, _, ok := st.MoveBy(traverseDir); ok {
						fmt.Printf("$%03X: edge $%04X %s\n", uint16(t.Supertile), uint16(se.c), traverseDir)
						q.SubmitJob(
							&ReachTask{
								InitialEmulator: e,
								EntranceID:      t.EntranceID,
								Rooms:           t.Rooms,
								RoomsLock:       t.RoomsLock,
								Supertile:       neighborSt,
								Start:           se.c.OppositeEdge(),
								Direction:       traverseDir,
							},
							ReachTaskInterRoom,
						)
						continue
					}
				}
			}

			if canTurn {
				// turn from here:
				if c, d, ok := room.AttemptTraversal(se.c, traverseDir.Opposite(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: 0})
				}
				if c, d, ok := room.AttemptTraversal(se.c, traverseDir.RotateCW(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: 0})
				}
				if c, d, ok := room.AttemptTraversal(se.c, traverseDir.RotateCCW(), traverseBy); ok {
					lifo = append(lifo, SE{c: c, d: d, s: 0})
				}
			}
			// traverse in the primary direction:
			if c, d, ok := room.AttemptTraversal(se.c, traverseDir, traverseBy); ok {
				lifo = append(lifo, SE{c: c, d: d, s: 0})
			}
		}
	}

	if room.Rendered == nil {
		// render BG layers:
		pal, bg1p, bg2p, addColor, halfColor := renderBGLayers(e.WRAM, e.VRAM[0x4000:0x8000])

		g := image.NewNRGBA(image.Rect(0, 0, 512, 512))
		ComposeToNonPaletted(g, pal, bg1p, bg2p, addColor, halfColor)

		// store full underworld rendering for inclusion into EG map:
		room.Rendered = g
	}

	return
}

func (room *RoomState) CanTraverseDir(c MapCoord, d Direction) bool {
	f := room.AllowDirFlags[c]
	return f&uint8(1<<d) != 0
}

func (room *RoomState) AttemptTraversal(c MapCoord, d Direction, by int) (nc MapCoord, nd Direction, ok bool) {
	if !room.CanTraverseDir(c, d) {
		nc, nd, ok = c, d, false
		return
	}

	nc, nd, ok = c.MoveBy(d, by)
	return
}
