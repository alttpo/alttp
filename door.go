package main

type Door struct {
	Type   DoorType  // $1980
	Pos    MapCoord  // $19A0
	Dir    Direction // $19C0
	IsExit bool      // found via array at $19E2 which points to Pos (after SHR 1)
}

func (d *Door) ContainsCoord(t MapCoord) bool {
	dl, dr, dc := d.Pos.RowCol()
	tl, tr, tc := t.RowCol()
	if tl != dl {
		return false
	}
	if tc < dc || tc >= dc+4 {
		return false
	}
	if tr < dr || dr >= dr+4 {
		return false
	}
	return true
}

func (d *Door) IsEdge() bool {
	ok, _, _, _ := d.Pos.IsDoorEdge()
	return ok
}
