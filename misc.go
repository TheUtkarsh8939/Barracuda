package main

import (
	"github.com/corentings/chess/v2"
)

// Move is a lightweight representation of a chess move (from-square, to-square).
// Used in the killer move table instead of *chess.Move to avoid the overhead
// of storing full move metadata for every killer.
type Move struct {
	square1 chess.Square
	square2 chess.Square
}

// SearchOptions holds the parameters parsed from a UCI "go" command.
// These control how deep/long the engine searches.
type SearchOptions struct {
	depth     uint8 // Fixed depth to search to
	blackTime int   // Black's remaining time in ms
	whiteTime int   // White's remaining time in ms
	moveTime  int   // Time allocated for this specific move in ms
	isInf     bool  // True if "go infinite" was received
	binc      int   // Black's time increment per move in ms
	winc      int   // White's time increment per move in ms
}

// InsertionSort sorts a move slice in-place using the provided comparator.
// Insertion sort is O(n²) but performs well on nearly-sorted data.
// This was written as an alternative to sort.Slice for benchmarking;
// currently sort.Slice is used in production (see search.go).
func InsertionSort(moves []*chess.Move, less func(a, b *chess.Move) bool) {
	n := len(moves)
	for i := 1; i < n; i++ {
		key := moves[i]
		j := i - 1
		// Shift elements forward until we find the correct position for key.
		for j >= 0 && less(key, moves[j]) {
			moves[j+1] = moves[j]
			j--
		}
		moves[j+1] = key
	}
}

// isCastlingMove returns true if the given move is a king castling move.
// Detected by checking if the king moves from its starting square to a known castling destination.
// White castles: E1→G1 (kingside) or E1→C1 (queenside)
// Black castles: E8→G8 (kingside) or E8→C8 (queenside)
func isCastlingMove(move *chess.Move) bool {
	kingStart := move.S1()
	kingEnd := move.S2()

	if kingStart == chess.E1 {
		if kingEnd == chess.G1 || kingEnd == chess.C1 {
			return true
		}
	}
	if kingStart == chess.E8 {
		if kingEnd == chess.G8 || kingEnd == chess.C8 {
			return true
		}
	}
	return false
}
