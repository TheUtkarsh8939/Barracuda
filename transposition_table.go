package main

// ttMask is used to compute the TT index via hash & ttMask (equivalent to hash % ttSize).
const ttMask = ttSize - 1

// ttEntry stores a cached position evaluation together with the search depth it was computed at.
// Entries are only returned when storedDepth >= requestedDepth, ensuring a shallow result
// is never substituted for a deeper one (which caused incorrect moves in earlier versions).
// The hashKey field stores the upper bits of the Zobrist hash for collision detection.
type ttEntry struct {
	hashKey uint64
	score   int
	depth   uint8
	bound   uint8
}

// transpositionTable caches position evaluations indexed by Zobrist hash.
// Array-based with modulo indexing for ~10x faster lookups than Go maps.
// Persists across iterative deepening iterations — depth validation keeps entries correct.
var transpositionTable [ttSize]ttEntry

// ttLookup probes the transposition table for a cached entry.
// Exact scores can be returned immediately. Bound entries are used to tighten
// the alpha-beta window and can trigger an immediate cutoff if the window closes.
func ttLookup(h uint64, depth uint8, alpha int, beta int) (int, int, int, bool) {
	idx := h & ttMask
	entry := &transpositionTable[idx]
	if entry.hashKey == h && entry.depth >= depth {
		switch entry.bound {
		case ttBoundExact:
			return entry.score, alpha, beta, true
		case ttBoundLower:
			if entry.score > alpha {
				alpha = entry.score
			}
		case ttBoundUpper:
			if entry.score < beta {
				beta = entry.score
			}
		}
		if alpha >= beta {
			return entry.score, alpha, beta, true
		}
	}
	return 0, alpha, beta, false
}

// ttStore saves an evaluation in the transposition table.
func ttStore(h uint64, score int, depth uint8, bound uint8) {
	idx := h & ttMask
	transpositionTable[idx] = ttEntry{hashKey: h, score: score, depth: depth, bound: bound}
}

// clearTT resets the transposition table for a new search.
func clearTT() {
	transpositionTable = [ttSize]ttEntry{}
}
