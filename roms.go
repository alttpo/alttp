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

	RoomData_PotItems_Pointers: 0x01_DB67,

	SpriteHitBox_OffsetXLow:  0x06_F735,
	SpriteHitBox_OffsetXHigh: 0x06_F735 + 0x20,
	SpriteHitBox_Width:       0x06_F735 + 0x40,
	SpriteHitBox_OffsetYLow:  0x06_F735 + 0x60,
	SpriteHitBox_OffsetYHigh: 0x06_F735 + 0x80,
	SpriteHitBox_Height:      0x06_F735 + 0xA0,
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

	RoomData_PotItems_Pointers: 0x01_DB69, // confirmed

	SpriteHitBox_OffsetXLow:  0x06_F72F, // confirmed
	SpriteHitBox_OffsetXHigh: 0x06_F72F + 0x20,
	SpriteHitBox_Width:       0x06_F72F + 0x40,
	SpriteHitBox_OffsetYLow:  0x06_F72F + 0x60,
	SpriteHitBox_OffsetYHigh: 0x06_F72F + 0x80,
	SpriteHitBox_Height:      0x06_F72F + 0xA0,
}
