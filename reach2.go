package main

import (
	"fmt"
	"image"
	"image/gif"
	"roomloader/taskqueue"
	"sync"
)

type ReachTask struct {
	EntranceID      uint8
	InitialEmulator *System

	Rooms     map[uint16]*RoomState
	RoomsLock *sync.Mutex
}

type T = *ReachTask
type Q = *taskqueue.Q[T]

func ReachTaskWorker(q Q, t T) {
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
	dungeonID := read8(wram, 0x040C)

	// read initial entrance supertile:
	st := read16(wram, 0xA0)

	fmt.Printf("entrance $%02X -> $%03X\n", t.EntranceID, st)

	t.RoomsLock.Lock()
	var room *RoomState
	room, ok := t.Rooms[st]
	if !ok {
		room = &RoomState{
			Supertile: Supertile(st),
			Entrance: &Entrance{
				EntranceID: t.EntranceID,
				Supertile:  Supertile(st),
				EntryCoord: 0,
				// Rooms:          []*RoomState{},
				// Supertiles:     map[Supertile]*RoomState{},
				// SupertilesLock: sync.Mutex{},
			},
			IsLoaded:          false,
			Rendered:          nil,
			GIF:               gif.GIF{},
			Animated:          gif.GIF{},
			AnimatedTileMap:   [][16384]byte{},
			AnimatedLayers:    []int{},
			AnimatedLayer:     0,
			EnemyMovementGIF:  gif.GIF{},
			EntryPoints:       []EntryPoint{},
			ExitPoints:        []ExitPoint{},
			WarpExitTo:        0,
			StairExitTo:       [4]Supertile{},
			WarpExitLayer:     0,
			StairTargetLayer:  [4]MapCoord{},
			Doors:             []Door{},
			Stairs:            []MapCoord{},
			SwapLayers:        map[MapCoord]struct{}{},
			TilesVisited:      map[MapCoord]struct{}{},
			TilesVisitedStar0: map[MapCoord]struct{}{},
			TilesVisitedStar1: map[MapCoord]struct{}{},
			TilesVisitedTag0:  map[MapCoord]struct{}{},
			TilesVisitedTag1:  map[MapCoord]struct{}{},
			Tiles:             [8192]byte{},
			Reachable:         [8192]byte{},
			Hookshot:          map[MapCoord]byte{},
			e:                 System{},
			WRAM:              [131072]byte{},
			WRAMAfterLoaded:   [131072]byte{},
			VRAMTileSet:       [16384]byte{},
			markedPit:         false,
			markedFloor:       false,
			lifoSpace:         [8192]ScanState{},
			lifo:              []ScanState{},
			HasReachablePit:   false,
		}
		t.Rooms[st] = room
		for i := range room.Reachable {
			room.Reachable[i] = 0x01
		}
	}
	t.RoomsLock.Unlock()

	// don't have two or more goroutines clobbering the same room:
	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	type SE struct {
		c MapCoord
		d Direction
		s int
	}
	lifo := make([]SE, 0, 1024)

	{
		// find entry point:
		y := read16(wram, 0x0020)
		x := read16(wram, 0x0022)
		lyr := read16(wram, 0x00EE)
		c := AbsToMapCoord(x, y, lyr)
		d := Direction(read8(wram, 0x002F) >> 1)

		room.Reachable[c] = 0xFF

		lifo = append(lifo, SE{c: c, d: d, s: 0})
	}

	tiles := wram[0x12000:0x14000]

	for len(lifo) > 0 {
		se := lifo[len(lifo)-1]
		lifo = lifo[0 : len(lifo)-1]

		fmt.Printf("$%03X: %02X\n", st, tiles[se.c])
	}

	{
		// render BG layers:
		pal, bg1p, bg2p, addColor, halfColor := renderBGLayers(e.WRAM, e.VRAM[0x4000:0x8000])

		g := image.NewNRGBA(image.Rect(0, 0, 512, 512))
		ComposeToNonPaletted(g, pal, bg1p, bg2p, addColor, halfColor)

		// store full underworld rendering for inclusion into EG map:
		room.Rendered = g
	}

	_ = st
	_ = dungeonID
}
