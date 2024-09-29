package main

import "fmt"

type DoorType uint8

const (
	DoorType00 = DoorType(0x00) // Normal door
	DoorType02 = DoorType(0x02) // Normal door (lower layer)
	DoorType04 = DoorType(0x04) // Exit (lower layer)
	DoorType06 = DoorType(0x06) // Unused cave exit (lower layer)
	DoorType08 = DoorType(0x08) // Waterfall door
	DoorType0A = DoorType(0x0A) // Fancy dungeon exit
	DoorType0C = DoorType(0x0C) // Fancy dungeon exit (lower layer)
	DoorType0E = DoorType(0x0E) // Cave exit
	DoorType10 = DoorType(0x10) // Lit cave exit (lower layer)
	DoorType12 = DoorType(0x12) // Exit marker
	DoorType14 = DoorType(0x14) // Dungeon swap marker
	DoorType16 = DoorType(0x16) // Layer swap marker
	DoorType18 = DoorType(0x18) // Double sided shutter door
	DoorType1A = DoorType(0x1A) // Eye watch door
	DoorType1C = DoorType(0x1C) // Small key door
	DoorType1E = DoorType(0x1E) // Big key door
	DoorType20 = DoorType(0x20) // Small key stairs (upwards)
	DoorType22 = DoorType(0x22) // Small key stairs (downwards)
	DoorType24 = DoorType(0x24) // Small key stairs (lower layer; upwards)
	DoorType26 = DoorType(0x26) // Small key stairs (lower layer; downwards)
	DoorType28 = DoorType(0x28) // Dash wall
	DoorType2A = DoorType(0x2A) // Bombable cave exit
	DoorType2C = DoorType(0x2C) // Unopenable, double-sided big key door
	DoorType2E = DoorType(0x2E) // Bombable door
	DoorType30 = DoorType(0x30) // Exploding wall
	DoorType32 = DoorType(0x32) // Curtain door
	DoorType34 = DoorType(0x34) // Unusable bottom-sided shutter door
	DoorType36 = DoorType(0x36) // Bottom-sided shutter door
	DoorType38 = DoorType(0x38) // Top-sided shutter door
	DoorType3A = DoorType(0x3A) // Unusable normal door (lower layer)
	DoorType3C = DoorType(0x3C) // Unusable normal door (lower layer)
	DoorType3E = DoorType(0x3E) // Unusable normal door (lower layer)
	DoorType40 = DoorType(0x40) // Normal door (lower layer; used with one-sided shutters)
	DoorType42 = DoorType(0x42) // Unused double-sided shutter
	DoorType44 = DoorType(0x44) // Double-sided shutter (lower layer)
	DoorType46 = DoorType(0x46) // Explicit room door
	DoorType48 = DoorType(0x48) // Bottom-sided shutter door (lower layer)
	DoorType4A = DoorType(0x4A) // Top-sided shutter door (lower layer)
	DoorType4C = DoorType(0x4C) // Unusable normal door (lower layer)
	DoorType4E = DoorType(0x4E) // Unusable normal door (lower layer)
	DoorType50 = DoorType(0x50) // Unusable normal door (lower layer)
	DoorType52 = DoorType(0x52) // Unusable bombed-open door (lower layer)
	DoorType54 = DoorType(0x54) // Unusable glitchy door (lower layer)
	DoorType56 = DoorType(0x56) // Unusable glitchy door (lower layer)
	DoorType58 = DoorType(0x58) // Unusable normal door (lower layer)
	DoorType5A = DoorType(0x5A) // Unusable glitchy/stairs up (lower layer)
	DoorType5C = DoorType(0x5C) // Unusable glitchy/stairs up (lower layer)
	DoorType5E = DoorType(0x5E) // Unusable glitchy/stairs up (lower layer)
	DoorType60 = DoorType(0x60) // Unusable glitchy/stairs up (lower layer)
	DoorType62 = DoorType(0x62) // Unusable glitchy/stairs down (lower layer)
	DoorType64 = DoorType(0x64) // Unusable glitchy/stairs up (lower layer)
	DoorType66 = DoorType(0x66) // Unusable glitchy/stairs down (lower layer)
)

func (t DoorType) IsEdgeDoorwayToNeighbor() bool {
	if t == 0x04 || t == 0x06 || t == 0x0A || t == 0x0C || t == 0x0E || t == 0x10 || t == 0x12 || t == 0x2A {
		// dungeon exits
		return false
	}
	if t >= 0x20 && t <= 0x27 {
		// doors to stairs
		return false
	}
	return true
}

func (t DoorType) IsOverworldExit() bool {
	if t >= 0x04 && t <= 0x06 {
		// exit door:
		return true
	}
	if t >= 0x0A && t <= 0x12 {
		// exit door:
		return true
	}
	//if t == 0x22 {
	//	// supertile 0b6, north key door covering stairwell?
	//	return true
	//}
	if t == 0x2A {
		// bombable cave exit:
		return true
	}
	//if t == 0x2E {
	//	// bombable door exit(?):
	//	return true
	//}
	return false
}

func (t DoorType) IsLayer2() bool {
	if t == 0x02 {
		return true
	}
	if t == 0x04 {
		return true
	}
	if t == 0x06 {
		return true
	}
	if t == 0x0C {
		return true
	}
	if t == 0x10 {
		return true
	}
	if t == 0x24 {
		return true
	}
	if t == 0x26 {
		return true
	}
	if t == 0x3A {
		return true
	}
	if t == 0x3C {
		return true
	}
	if t == 0x3E {
		return true
	}
	if t == 0x40 {
		return true
	}
	if t == 0x44 {
		return true
	}
	if t >= 0x48 && t <= 0x66 {
		return true
	}
	return false
}

func (t DoorType) IsStairwell() bool {
	return t >= 0x20 && t <= 0x26
}

func (t DoorType) String() string {
	return fmt.Sprintf("$%02x", uint8(t))
}
