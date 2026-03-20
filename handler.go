package main

// handler.go contains killer-move table utilities used by search ordering.

// maxKillerDepth is the maximum depth for killer move storage.
// Using a fixed-size array avoids map hashing overhead for killer lookups.
const maxKillerDepth = 64

// killerEntry stores up to 2 killer moves for a given depth.
type killerEntry struct {
	moves [2]Move
	count uint8
}

// killerTable is a fixed-size array of killer moves indexed by depth.
// Replaces the previous map[uint8][]Move for faster access.
var killerTable [maxKillerDepth]killerEntry

// storeKillerMove records a move that caused a beta cutoff (a "killer move") at a given depth.
// We keep at most 2 killers per depth; when a new killer arrives, the older one is shifted out.
// These are searched early in sibling nodes at the same depth since they often cause cutoffs there too.
func storeKillerMove(depth uint8, move Move) {
	if depth >= maxKillerDepth {
		return
	}
	entry := &killerTable[depth]
	if entry.count < 2 {
		entry.moves[entry.count] = move
		entry.count++
	} else {
		entry.moves[1] = entry.moves[0] // Demote slot 0 → slot 1
		entry.moves[0] = move           // New killer takes slot 0
	}
}

// getKillerMoves returns the killer moves for a given depth and count of stored killers.
func getKillerMoves(depth uint8) (Move, Move, uint8) {
	if depth >= maxKillerDepth {
		return Move{}, Move{}, 0
	}
	entry := &killerTable[depth]
	return entry.moves[0], entry.moves[1], entry.count
}

// clearKillerTable resets the killer move table for a new search.
func clearKillerTable() {
	killerTable = [maxKillerDepth]killerEntry{}
}
