package main

import (
	"fmt"
	"image"
	"image/gif"
	"roomloader/taskqueue"
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

	// iteratively recurse over processing stack:
	for len(lifo) > 0 {
		se := lifo[len(lifo)-1]
		lifo = lifo[0 : len(lifo)-1]

		if _, ok := room.TilesVisited[se.c]; ok {
			continue
		}
		room.TilesVisited[se.c] = empty{}

		v := tiles[se.c]
		room.Reachable[se.c] = v
		fmt.Printf("$%03X: [$%04X]=%02X\n", uint16(st), uint16(se.c), v)

		// transition to neighboring room at the edges:
		if ok, edgeDir, _, _ := se.c.IsEdge(); ok {
			if neighborSt, _, ok := st.MoveBy(edgeDir); ok {
				q.SubmitJob(
					&ReachTask{
						InitialEmulator: e,
						EntranceID:      t.EntranceID,
						Rooms:           t.Rooms,
						RoomsLock:       t.RoomsLock,
						Supertile:       neighborSt,
						Start:           se.c.OppositeEdge(),
						Direction:       edgeDir,
					},
					ReachTaskInterRoom,
				)
			}
		}

		canTraverse := false
		if v == 0x20 {
			room.HasReachablePit = true
		} else if room.isAlwaysWalkable(v) || room.isMaybeWalkable(se.c, v) {
			canTraverse = true
		} else if v&0xF0 == 0x80 {
			// shutter doors
			canTraverse = true
		} else if v&0xF0 == 0xF0 {
			// entrances:
			canTraverse = true
		}

		if canTraverse {
			// traverse in 4 directions from here:
			if c, d, ok := se.c.MoveBy(se.d.Opposite(), 1); ok {
				lifo = append(lifo, SE{c: c, d: d, s: 0})
			}
			if c, d, ok := se.c.MoveBy(se.d.RotateCW(), 1); ok {
				lifo = append(lifo, SE{c: c, d: d, s: 0})
			}
			if c, d, ok := se.c.MoveBy(se.d.RotateCCW(), 1); ok {
				lifo = append(lifo, SE{c: c, d: d, s: 0})
			}
			if c, d, ok := se.c.MoveBy(se.d, 1); ok {
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
	}

	wram := (e.WRAM)[:]
	tiles := wram[0x12000:0x14000]

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

	return
}
