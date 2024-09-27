package main

var alttpJP10 = romPointers{
	Module_MainRouting:                                                     0x00_80B5,
	Underworld_LoadRoom:                                                    0x01_873A,
	Underworld_LoadCustomTileAttributes:                                    0x0F_FD65,
	Underworld_LoadAttributeTable:                                          0x01_B8BF,
	Underworld_LoadEntrance_DoPotsBlocksTorches:                            0x02_D854,
	Module06_UnderworldLoad_after_JSR_Underworld_LoadEntrance:              0x02_8157,
	LoadDefaultTileTypes:                                                   0x0F_FD2A,
	Intro_InitializeDefaultGFX:                                             0x0C_C208,
	Intro_InitializeDefaultGFX_after_JSL_DecompressAnimatedUnderworldTiles: 0x0C_C237,
	Intro_CreateTextPointers:                                               0x02_8022,
	DecompressFontGFX:                                                      0x0E_F572,
	LoadItemGFXIntoWRAM4BPPBuffer:                                          0x00_D271,
	InitializeSaveFile:                                                     0x0C_DB3E,
	CopySaveToWRAM:                                                         0x0C_CEB2,
	Ancilla_TerminateSelectInteractives:                                    0x09_AC57,
	NMI_PrepareSprites:                                                     0x00_85FC,
	NMI_DoUpdates:                                                          0x00_89E0,
	NMI_ReadJoypads:                                                        0x00_83D1,
	ClearOAMBuffer:                                                         0x00_841E,
	Underworld_HandleRoomTags:                                              0x01_C2FD,

	Patch_JSR_Underworld_LoadSongBankIfNeeded: 0x02_8293,
	Patch_SEP_20_RTL:          0x02_82BC,
	Patch_RebuildHUD_Keys:     0x0D_FA88,
	Patch_Sprite_PrepOAMCoord: 0x06_E48B,
	Patch_LoadSongBank:        0x00_8888,

	Reveal_PotItems:            0x01_E6B0,
	RoomData_PotItems_Pointers: 0x01_DB67,

	SpriteHitBox_OffsetXLow:  0x06_F735,
	SpriteHitBox_OffsetXHigh: 0x06_F735 + 0x20,
	SpriteHitBox_Width:       0x06_F735 + 0x40,
	SpriteHitBox_OffsetYLow:  0x06_F735 + 0x60,
	SpriteHitBox_OffsetYHigh: 0x06_F735 + 0x80,
	SpriteHitBox_Height:      0x06_F735 + 0xA0,

	ExtractPointers: func(p *romPointers, e *System) {
		extractRoomData_PotItems_Pointers(p, e)
	},
}

var alttpUS = romPointers{
	Module_MainRouting:                                                     0x00_80B5, // confirmed
	Underworld_LoadRoom:                                                    0x01_873A, // confirmed
	Underworld_LoadCustomTileAttributes:                                    0x0E_942A, // confirmed; renamed to Underworld_LoadCustomTileTypes
	Underworld_LoadAttributeTable:                                          0x01_B8BF, // confirmed
	Underworld_LoadEntrance_DoPotsBlocksTorches:                            0x02_DAF0, // confirmed; offset to PHB after the PLB
	Module06_UnderworldLoad_after_JSR_Underworld_LoadEntrance:              0x02_824D, // confirmed
	LoadDefaultTileTypes:                                                   0x0E_97D9, // confirmed
	Intro_InitializeDefaultGFX:                                             0x0C_C1F9, // confirmed
	Intro_InitializeDefaultGFX_after_JSL_DecompressAnimatedUnderworldTiles: 0x0C_C228, // confirmed
	Intro_CreateTextPointers:                                               0x0E_D3EB, // address confirmed, confirm renamed to CreateMessagePointers??
	DecompressFontGFX:                                                      0,         // removed
	LoadItemGFXIntoWRAM4BPPBuffer:                                          0x00_D231, // confirmed
	InitializeSaveFile:                                                     0x0C_DBDC, // confirmed
	CopySaveToWRAM:                                                         0x0C_CFBB, // confirmed
	Ancilla_TerminateSelectInteractives:                                    0x09_AC6B, // confirmed
	NMI_PrepareSprites:                                                     0x00_85FC, // confirmed
	NMI_DoUpdates:                                                          0x00_89E0, // confirmed
	NMI_ReadJoypads:                                                        0x00_83D1, // confirmed
	ClearOAMBuffer:                                                         0x00_841E, // confirmed
	Underworld_HandleRoomTags:                                              0x01_C2FD, // confirmed

	Patch_JSR_Underworld_LoadSongBankIfNeeded: 0x02_8389, // confirmed
	Patch_SEP_20_RTL:          0x02_83B2, // confirmed
	Patch_RebuildHUD_Keys:     0x0D_FA68, // confirmed
	Patch_Sprite_PrepOAMCoord: 0x06_E485, // confirmed
	Patch_LoadSongBank:        0x00_8888, // confirmed

	Reveal_PotItems:            0x01_E6B2, // confirmed
	RoomData_PotItems_Pointers: 0x01_DB69, // confirmed

	SpriteHitBox_OffsetXLow:  0x06_F72F, // confirmed
	SpriteHitBox_OffsetXHigh: 0x06_F72F + 0x20,
	SpriteHitBox_Width:       0x06_F72F + 0x40,
	SpriteHitBox_OffsetYLow:  0x06_F72F + 0x60,
	SpriteHitBox_OffsetYHigh: 0x06_F72F + 0x80,
	SpriteHitBox_Height:      0x06_F72F + 0xA0,

	ExtractPointers: func(p *romPointers, e *System) {
		extractRoomData_PotItems_Pointers(p, e)
	},
}

func readBusChunk(e *System, addr uint32, into []byte) {
	size := uint32(len(into))

	readFn := e.Bus.Read[addr>>4]
	for i := uint32(0); i < size; addr, i = addr+1, i+1 {
		if addr&3 == 0 {
			readFn = e.Bus.Read[addr>>4]
		}
		into[i] = readFn(addr)
	}
}

func extractRoomData_PotItems_Pointers(p *romPointers, e *System) {
	// RevealPotItem: ; JP 1.0
	// #_01E6B0: STA.b $04
	// #_01E6B2: LDA.w $0B9C
	// #_01E6B5: AND.w #$FF00
	// #_01E6B8: STA.w $0B9C
	// #_01E6BB: LDA.b $A0
	// #_01E6BD: ASL A
	// #_01E6BE: TAX
	// #_01E6BF: LDA.l RoomData_PotItems_Pointers,X
	// #_01E6C3: STA.b $00
	// #_01E6C5: LDA.w #RoomData_PotItems_Pointers>>16
	// #_01E6C8: STA.b $02
	// #_01E6CA: LDY.w #$FFFD
	// #_01E6CD: LDX.w #$FFFF

	// verify first 16 bytes of code:
	c0 := [16]byte{}
	readBusChunk(e, p.Reveal_PotItems, c0[:])
	if c0 != [16]byte{
		0x85, 0x04,
		0xAD, 0x9C, 0x0B,
		0x29, 0x00, 0xFF,
		0x8D, 0x9C, 0x0B,
		0xA5, 0xA0,
		0x0A,
		0xAA,
		0xBF} {
		return
	}

	// read 3-byte pointer we're interested in:
	lhb := [3]byte{}
	readBusChunk(e, p.Reveal_PotItems+0x10, lhb[:])
	p.RoomData_PotItems_Pointers = uint32(lhb[0]) | uint32(lhb[1])<<8 | uint32(lhb[2])<<16
}
