package main

import "math/bits"

//Takes Rooks of a colour and pawn bitboard on open files and returns a score based on how many rooks are on open files.
// Each rook on an open file gets +10
// This encourages the engine to place rooks on open files where they can be more active and exert pressure on the opponent's position.
func rookOnOpenFiles(pawnBitboard uint64, rookBitboard uint64) int {
	if pawnBitboard == 0 {
		return 10 * bits.OnesCount64(rookBitboard) // If there are no pawns, all files are open, so each rook gets the full 10 points.
	}
	score := 0
	for i := 0; i < 8; i++ {
		masked := pawnBitboard & fileMasks[i]
		if masked == 0 {
			// Open file
			score += 10 * bits.OnesCount64(rookBitboard&fileMasks[i])
		}
	}
	return score
}

//Takes King of a colour and pawn bitboard on open files and returns a score based on if the king is near an open file.
// If the king is on a square adjacent to an open file, it gets -5. If king is on an open file, it gets -10. This encourages the engine to keep the king away from open files where it can be more vulnerable to attacks
func kingNearOpenFiles(pawnBitboard uint64, kingBitboard uint64) int {
	if kingBitboard == 0 {
		return 0
	}

	kingSq := bits.TrailingZeros64(kingBitboard)
	kingFile := kingSq % 8

	if pawnBitboard&fileMasks[kingFile] == 0 {
		return -10 // King is on an open file.
	}

	if kingFile > 0 && pawnBitboard&fileMasks[kingFile-1] == 0 {
		return -5 // King is adjacent to an open file on the left.
	}
	if kingFile < 7 && pawnBitboard&fileMasks[kingFile+1] == 0 {
		return -5 // King is adjacent to an open file on the right.
	}

	return 0
}
