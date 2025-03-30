package main

import (
	"github.com/corentings/chess"
)

type Move struct {
	square1 chess.Square
	square2 chess.Square
}

func InsertionSort(moves []*chess.Move, less func(a, b *chess.Move) bool) {
	n := len(moves)
	for i := 1; i < n; i++ {
		key := moves[i]
		j := i - 1

		// Move elements that are greater than key one position ahead
		for j >= 0 && less(key, moves[j]) {
			moves[j+1] = moves[j]
			j--
		}
		moves[j+1] = key
	}
}

// isCastlingMove determines if a given move is a castling move.
func isCastlingMove(move *chess.Move) bool {
	kingStart := move.S1() // Starting square of the move
	kingEnd := move.S2()   // Ending square of the move

	// Define king's starting positions for both colors
	whiteKingStart := chess.E1
	blackKingStart := chess.E8

	// Check for White's castling moves
	if kingStart == whiteKingStart {
		if kingEnd == chess.G1 || kingEnd == chess.C1 { // G1 = kingside, C1 = queenside
			return true
		}
	}

	// Check for Black's castling moves
	if kingStart == blackKingStart {
		if kingEnd == chess.G8 || kingEnd == chess.C8 { // G8 = kingside, C8 = queenside
			return true
		}
	}

	return false
}
