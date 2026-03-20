// pv_store.go — Principal Variation (PV) table for Barracuda's search.
//
// The principal variation is the sequence of best moves the engine predicts for both sides
// from the current root position. It is sent to the GUI via "info ... pv" lines and helps
// the user (and developer) understand the engine's reasoning.
//
// Design:
//   - pvTable mirrors the transposition table in size and indexing (hash & ttMask).
//   - pvLookup / pvStore record the best move for each position at each depth.
//   - buildPVLine walks the table from the root, following best-move links, to reconstruct
//     the full predicted line. It re-hashes each successor position using fastPosHash.
//
// Why separate from the TT?
//
//	Sharing PV and TT space is common but causes PV entries to be overwritten by interior
//	nodes. A dedicated pvTable ensures buildPVLine can always recover a complete line at
//	the searched depth, even in positions with heavy TT traffic.
package main

import (
	"fmt"

	"github.com/corentings/chess/v2"
)

// pvEntry stores the best move found for a position at a given depth.
// This allows reconstructing the principal variation (predicted best move chain)
// from the root down to the searched leaf.
type pvEntry struct {
	hashKey uint64
	depth   uint8
	moveUCI string
}

// pvTable is an array-based principal variation table indexed exactly like TT.
var pvTable [ttSize]pvEntry

// lastPrincipalVariation stores the latest full best line from root to leaf.
var lastPrincipalVariation []string

// predictedPVByHash maps a position hash to the PV move predicted from that position.
// It is rebuilt from the latest completed PV line and reused by the next search.
var predictedPVByHash = make(map[uint64]Move)

// pvLookup returns the cached best move for this position if available at sufficient depth.
func pvLookup(h uint64, depth uint8) (string, bool) {
	idx := h & ttMask
	entry := &pvTable[idx]
	if entry.hashKey == h && entry.depth >= depth && entry.moveUCI != "" {
		return entry.moveUCI, true
	}
	return "", false
}

// pvStore saves the best move found at this node for PV reconstruction.
func pvStore(h uint64, depth uint8, move *chess.Move) {
	if move == nil {
		return
	}
	idx := h & ttMask
	pvTable[idx] = pvEntry{hashKey: h, depth: depth, moveUCI: fmt.Sprint(move)}
}

// clearPV resets the per-search PV table/line state.
// The predictedPVByHash map is updated separately after completed iterations.
func clearPV() {
	pvTable = [ttSize]pvEntry{}
	lastPrincipalVariation = nil
}

// pvPredictedMove returns the previously predicted PV move for this position hash.
func pvPredictedMove(h uint64) (Move, bool) {
	m, ok := predictedPVByHash[h]
	return m, ok
}

// updatePredictedPVFromLine rebuilds predictedPVByHash from a root position and PV UCI line.
// This enables PV-follow ordering across moves: if the game follows the expected line,
// the matching move is searched first with a strong ordering bonus.
func updatePredictedPVFromLine(position *chess.Position, line []string) {
	predictedPVByHash = make(map[uint64]Move, len(line))
	current := position

	for _, moveUCI := range line {
		move, err := chess.Notation.Decode(chess.UCINotation{}, current, moveUCI)
		if err != nil {
			break
		}

		predictedPVByHash[fastPosHash(current)] = Move{move.S1(), move.S2()}
		current = current.Update(move)
		if current.Status() != chess.NoMethod {
			break
		}
	}
}

// buildPVLine reconstructs the predicted best line from root at the given depth.
func buildPVLine(position *chess.Position, depth uint8) []string {
	line := make([]string, 0, depth)
	current := position

	for ply := uint8(0); ply < depth; ply++ {
		remaining := depth - ply
		moveUCI, ok := pvLookup(fastPosHash(current), remaining)
		if !ok {
			break
		}

		line = append(line, moveUCI)
		move, err := chess.Notation.Decode(chess.UCINotation{}, current, moveUCI)
		if err != nil {
			break
		}

		current = current.Update(move)
		if current.Status() != chess.NoMethod {
			break
		}
	}

	return line
}
