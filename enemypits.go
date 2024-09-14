package main

import (
	"fmt"
	"os"
)

func roomFindReachablePitsFromEnemies(room *RoomState) {
	st := room.Supertile

	e := &room.e
	wram := e.WRAM[:]
	// vram := e.VRAM[:]

	tiles := wram[0x12000:0x14000]

	coordLifo := make([]MapCoord, 0, 0x2000)

	for i := uint32(0); i < 16; i++ {
		// skip inactive enemies:
		isDead := read8(wram, uint32(0x0DD0+i)) == 0
		if isDead {
			continue
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
	}

	room.TilesVisited = make(map[MapCoord]empty, 0x2000)

	// make a map full of $01 Collision and carve out reachable areas:
	for i := range room.Reachable {
		room.Reachable[i] = 0x01
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

	if hasReachablePit {
		fmt.Printf("%s has reachable pit\n", st)
	}

	os.WriteFile(fmt.Sprintf("t%03X.tmap", uint16(st)), tiles, 0644)
	room.DrawSupertile()
}
