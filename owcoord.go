package main

type OWCoord uint16

func Map16ToOWCoord(t uint16) (c OWCoord) {
	y := t & 0xFF80
	x := t & 0x007F
	// y-coordinate must be doubled due to 2x2 expansion from map16 to map8
	// x-coordinate needs no transform because uint16->uint8 conversion is balanced by 2x2 expansion
	return OWCoord((y << 1) + x)
}
