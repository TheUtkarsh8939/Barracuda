package main

// storeKillerMove records a move that caused a beta cutoff (a "killer move") at a given depth.
// We keep at most 2 killers per depth; when a new killer arrives, the older one is shifted out.
// These are searched early in sibling nodes at the same depth since they often cause cutoffs there too.
func storeKillerMove(depth uint8, move Move) {
	if len(killerMoveTable[depth]) < 2 {
		killerMoveTable[depth] = append(killerMoveTable[depth], move)
	} else {
		killerMoveTable[depth][1] = killerMoveTable[depth][0] // Demote slot 0 → slot 1
		killerMoveTable[depth][0] = move                      // New killer takes slot 0
	}
}

// resetKillerMoveTable shifts the killer move table by 2 depth levels after iterative deepening
// moves to the next iteration. Killers from depth N in the previous search map to depth N+2
// in the new search (since the root is now 2 plies deeper), preserving their usefulness.
func resetKillerMoveTable(table map[uint8][]Move) map[uint8][]Move {
	if len(table) < 3 {
		return table
	}
	ntTable := make(map[uint8][]Move)
	for i := uint8(2); i < uint8(len(table)); i++ {
		ntTable[i] = table[i-2]
	}
	return ntTable
}
