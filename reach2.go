package main

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
	"roomloader/taskqueue"
	"strings"
	"sync"

	"golang.org/x/image/math/fixed"
)

type SE struct {
	c MapCoord
	d Direction
	s int
}

type ReachTask struct {
	InitialEmulator *System

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
	copy((*room.e.WRAM)[:], room.WRAM[:])
	copy((*room.e.VRAM)[:], room.VRAMTileSet[:])
	t.RoomsLock.Unlock()

	reachTaskFloodfill(q, t, room)
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

	t.SE.c = linkC
	t.SE.d = linkD
	t.SE.s = 0

	room := getOrCreateRoom(t, e)

	reachTaskFloodfill(q, t, room)

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

	// do first-time room processing work:
	// fmt.Printf("$%03X: room init\n", uint16(t.Supertile))

	wram := (*e.WRAM)[:]
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

	// this code is redundant to the tiles found within doorways, i hope...
	// // find layer-swap tiles in doorways:
	// swapCount := read16(wram, 0x044E)
	// room.SwapLayers = make(map[MapCoord]empty, swapCount*4)
	// for i := uint16(0); i < swapCount; i += 2 {
	// 	c := MapCoord(read16(wram, uint32(0x06C0+i)))

	// 	fmt.Printf("$%03X: swap %s\n", uint16(t.Supertile), c)
	// 	// mark the 2x2 tile as a layer-swap:
	// 	room.SwapLayers[c+0x00] = empty{}
	// 	room.SwapLayers[c+0x01] = empty{}
	// 	room.SwapLayers[c+0x40] = empty{}
	// 	room.SwapLayers[c+0x41] = empty{}
	// 	// have to put it on both layers? ew
	// 	room.SwapLayers[c|0x1000+0x00] = empty{}
	// 	room.SwapLayers[c|0x1000+0x01] = empty{}
	// 	room.SwapLayers[c|0x1000+0x40] = empty{}
	// 	room.SwapLayers[c|0x1000+0x41] = empty{}
	// }

	os.WriteFile(fmt.Sprintf("r%03X.post.tmap", uint16(t.Supertile)), tiles, 0644)
	os.WriteFile(fmt.Sprintf("r%03X.dir.tmap", uint16(t.Supertile)), room.AllowDirFlags[:], 0644)

	{
		// render BG layers:
		pal, bg1p, bg2p, addColor, halfColor := renderBGLayers(e.WRAM, e.VRAM[0x4000:0x8000])

		g := image.NewNRGBA(image.Rect(0, 0, 512, 512))
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

		// store full underworld rendering for inclusion into EG map:
		room.Rendered = g
		room.RenderedNRGBA = g
	}

	copy(room.WRAM[:], (*e.WRAM)[:])
	copy(room.VRAMTileSet[:], (*e.VRAM)[:])

	return
}

func tileAllowableDirFlags(v uint8) uint8 {
	// north/south doorways:
	if v == 0x80 || v == 0x82 || v == 0x84 || v == 0x86 || v == 0x8E || v == 0x8F || v == 0xA0 ||
		v == 0x5E || v == 0x5F || v&0xF8 == 0x30 || v == 0x38 || v == 0x39 {
		return 0b0000_0011
	}
	// east/west doorways:
	if v == 0x81 || v == 0x83 || v == 0x85 || v == 0x87 || v == 0x89 {
		return 0b0000_1100
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

	wram := room.WRAM
	tiles := wram[0x12000:0x14000]

	// push starting state:
	lifo := make([]SE, 0, 1024)
	lifo = append(lifo, t.SE)

	fmt.Printf("$%03X: start=%04X dir=%s\n", uint16(t.Supertile), uint16(t.SE.c), t.SE.d)

	// iteratively recurse over processing stack:
	for len(lifo) > 0 {
		se := lifo[len(lifo)-1]
		lifo = lifo[0 : len(lifo)-1]

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

		c := se.c
		v := tiles[c]

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
					q.SubmitJob(
						&ReachTask{
							InitialEmulator: room.e,
							EntranceID:      t.EntranceID,
							Rooms:           t.Rooms,
							RoomsLock:       t.RoomsLock,
							Supertile:       neighborSt,
							SE: SE{
								c: c.OppositeEdge() ^ layerSwap,
								d: traverseDir,
								s: se.s,
							},
						},
						ReachTaskInterRoom,
					)
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
				q.SubmitJob(
					&ReachTask{
						InitialEmulator: room.e,
						EntranceID:      t.EntranceID,
						Rooms:           t.Rooms,
						RoomsLock:       t.RoomsLock,
						Supertile:       neighborSt,
						SE: SE{
							c: ct,
							d: traverseDir,
							s: se.s,
						},
					},
					ReachTaskInterRoom,
				)
				continue
			}
		} else if v == 0x4B {
			// 4B - warp tile
			neighborSt := room.WarpExitTo
			ct := c | room.WarpExitLayer
			fmt.Printf("$%03X: warp $%04X exit to $%03X at %04X\n", uint16(t.Supertile), uint16(c), uint16(neighborSt), uint16(ct))
			q.SubmitJob(
				&ReachTask{
					InitialEmulator: room.e,
					EntranceID:      t.EntranceID,
					Rooms:           t.Rooms,
					RoomsLock:       t.RoomsLock,
					Supertile:       neighborSt,
					SE: SE{
						c: ct,
						d: traverseDir,
						s: se.s,
					},
				},
				ReachTaskInterRoom,
			)
			canTraverse = true
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
		} else if v == 0x20 {
			// pit:
			room.Reachable[c] = v
			room.HasReachablePit = true
		} else if v&0xF0 == 0x80 {
			// shutter doors and entrance doors
			canTraverse = true
			canTurn = false
		} else if v&0xF0 == 0xF0 {
			// doorways:
			canTraverse = true
			canTurn = false
		} else if room.isAlwaysWalkable(v) || room.isMaybeWalkable(c, v) {
			canTraverse = true
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
					q.SubmitJob(
						&ReachTask{
							InitialEmulator: room.e,
							EntranceID:      t.EntranceID,
							Rooms:           t.Rooms,
							RoomsLock:       t.RoomsLock,
							Supertile:       neighborSt,
							SE: SE{
								c: c.OppositeEdge() ^ layerSwap,
								d: traverseDir,
								s: se.s,
							},
						},
						ReachTaskInterRoom,
					)
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

func (room *RoomState) CanTraverseDir(c MapCoord, d Direction) bool {
	f := room.AllowDirFlags[c]
	return f&uint8(1<<d) != 0
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
