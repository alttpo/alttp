package main

type OWCoord uint16

// // obsolete: prefer to use Area.Traverse()
// func (c OWCoord) MoveBy(dir Direction, increment int) (OWCoord, Direction, bool) {
// 	it := int(c)
// 	row := (it & 0xF80) >> 7
// 	col := it & 0x7F

// 	// don't allow perpendicular movement along the outer edge
// 	// this prevents accidental/leaky flood fill along the edges
// 	if (col <= 1 || col >= 0x3E) && (dir != DirEast && dir != DirWest) {
// 		return c, dir, false
// 	} else if (row <= 1 || row >= 0x3E) && (dir != DirNorth && dir != DirSouth) {
// 		return c, dir, false
// 	}

// 	switch dir {
// 	case DirNorth:
// 		if row >= 0+increment {
// 			return OWCoord(it - (increment << 7)), dir, true
// 		}
// 		return c, dir, false
// 	case DirSouth:
// 		if row <= 0x7F-increment {
// 			return OWCoord(it + (increment << 7)), dir, true
// 		}
// 		return c, dir, false
// 	case DirWest:
// 		if col >= 0+increment {
// 			return OWCoord(it - increment), dir, true
// 		}
// 		return c, dir, false
// 	case DirEast:
// 		if col <= 0x7F-increment {
// 			return OWCoord(it + increment), dir, true
// 		}
// 		return c, dir, false
// 	default:
// 		panic("bad direction")
// 	}
// }
