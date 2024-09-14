package main

import (
	"fmt"
	"os"
	"strings"
)

func roomFindReachablePitsFromEnemies(room *RoomState) {
	st := room.Supertile

	e := &room.e
	wram := e.WRAM[:]
	// vram := e.VRAM[:]

	tiles := wram[0x12000:0x14000]

	coordLifo := make([]MapCoord, 0, 0x2000)
	startCoords := make([]MapCoord, 0, 16)

	for i := uint32(0); i < 16; i++ {
		// skip inactive enemies:
		isDead := read8(wram, uint32(0x0DD0+i)) == 0
		if isDead {
			continue
		}

		// skip non-enemies:
		et := read8(wram, uint32(0x0E20+i))
		switch et {
		// pipes:
		case 0xAE, 0xAF, 0xB0, 0xB1:
			continue
			//case
		}

		// find abs coords for this enemy:
		yl, yh := read8(wram, 0x0D00+i), read8(wram, 0x0D20+i)
		xl, xh := read8(wram, 0x0D10+i), read8(wram, 0x0D30+i)
		y := uint16(yl) | uint16(yh)<<8
		x := uint16(xl) | uint16(xh)<<8

		// which layer the sprite is on (0 or 1 hopefully):
		layer := read8(wram, 0x0F20+i)
		if layer > 1 {
			fmt.Printf("!!!! layer = %02X\n", layer)
		}
		layer = layer & 1

		// find tilemap coords for this enemy:
		coord := AbsToMapCoord(x, y, uint16(layer))
		// _, row, col := coord.RowCol()

		coordLifo = append(coordLifo, coord)
		startCoords = append(startCoords, coord)
	}

	room.TilesVisited = make(map[MapCoord]empty, 0x2000)

	// make a map full of $01 Collision and carve out reachable areas:
	for i := range room.Reachable {
		room.Reachable[i] = 0x01
	}

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
			//fmt.Printf("    door stop at marker\n")
			break
		}

		door := Door{
			Pos:  MapCoord(tpos >> 1),
			Type: DoorType(read16(wram, uint32(0x1980+(m<<1)))),
			Dir:  Direction(read16(wram, uint32(0x19C0+(m<<1)))),
		}
		room.Doors = append(room.Doors, door)

		switch door.Dir {
		case DirNorth:
		case DirSouth:
		case DirEast:
		case DirWest:
			var ok bool
			var c MapCoord

			// find how far to clear to opposite doorway:
			count := 3
			c = door.Pos
			dbg := strings.Builder{}
			for i := 0; i < 16; i++ {
				c, _, ok = c.MoveBy(DirWest, 1)
				if !ok {
					break
				}
				fmt.Fprintf(&dbg, "%02X,", tiles[c])
				if tiles[c] == 0x02 {
					count++
					break
				}
				count++
			}
			fmt.Printf("%s: %s: %s\n", st, door.Pos, dbg.String())

			// clear a path to the west:
			c, _, _ = door.Pos.MoveBy(DirSouth, 1)
			c, _, ok = c.MoveBy(DirEast, 2)
			for i := 0; ok && i < count; i++ {
				tiles[c+0x00] = 0
				tiles[c+0x40] = 0
				c, _, ok = c.MoveBy(DirWest, 1)
			}
		}
	}

	hasReachablePit := false
	for len(coordLifo) > 0 {
		lifoLen := len(coordLifo) - 1
		c := coordLifo[lifoLen]
		coordLifo = coordLifo[:lifoLen]

		// skip already visited tiles:
		if _, ok := room.TilesVisited[c]; ok {
			continue
		}
		// mark visited:
		room.TilesVisited[c] = empty{}

		t := tiles[c]

		if t == 0x20 {
			hasReachablePit = true
			room.Reachable[c] = t
		} else if room.isAlwaysWalkable(t) || room.isMaybeWalkable(c, t) {
			room.Reachable[c] = t
			if cn, _, ok := c.MoveBy(DirEast, 1); ok {
				coordLifo = append(coordLifo, cn)
			}
			if cn, _, ok := c.MoveBy(DirWest, 1); ok {
				coordLifo = append(coordLifo, cn)
			}
			if cn, _, ok := c.MoveBy(DirNorth, 1); ok {
				coordLifo = append(coordLifo, cn)
			}
			if cn, _, ok := c.MoveBy(DirSouth, 1); ok {
				coordLifo = append(coordLifo, cn)
			}
		}
	}

	// mark the enemy starting positions:
	for _, c := range startCoords {
		room.Reachable[c] = 0xFF
	}
	// mark doors:
	for _, door := range room.Doors {
		switch door.Dir {
		case DirNorth:
			room.Reachable[door.Pos] = 0xF0
		case DirSouth:
			room.Reachable[door.Pos] = 0xF1
		case DirEast:
			room.Reachable[door.Pos] = 0xF2
		case DirWest:
			room.Reachable[door.Pos] = 0xF3
		}
	}

	if hasReachablePit {
		fmt.Printf("%s has reachable pit\n", st)
	}

	os.WriteFile(fmt.Sprintf("t%03X.tmap", uint16(st)), tiles, 0644)
	room.DrawSupertile()
}
