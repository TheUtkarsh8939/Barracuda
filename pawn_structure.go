package main

import "math/bits"

// Binary Masks to get files from bitboards
var fileMasks = [8]uint64{
	0b000000010000000100000001000000010000000100000001000000010,
	0b000000100000001000000010000000100000001000000010000000100,
	0b000001000000010000000100000001000000010000000100000001000,
	0b000010000000100000001000000010000000100000001000000010000,
	0b000100000001000000010000000100000001000000010000000100000,
	0b001000000010000000100000001000000010000000100000001000000,
	0b010000000100000001000000010000000100000001000000010000000,
	0b100000001000000010000000100000001000000010000000100000001,
}

// pawnStructure evaluates structural pawn features from one side's pawn bitboard.
// Penalties: doubled pawns, weak file support; Bonus: pawn chains.
// Higher is better for that side.
func pawnStructure(pawnBitboard uint64) int {
	score := 0
	var pawnsPerFile [8]int
	for i := range pawnsPerFile {
		masked := pawnBitboard & fileMasks[i]

		pawnsPerFile[i] = bits.OnesCount64(masked)
	}

	for i, count := range pawnsPerFile {
		if count > 1 {
			score -= (count - 1) * 2 // Each extra pawn on the same file costs -2.
		}
		// Penalty for isolated/half-open files (adjacent file has no pawn).
		if i > 0 && pawnsPerFile[i-1] == 0 {
			score--
		}
		if i < 7 && pawnsPerFile[i+1] == 0 {
			score--
		}
	}

	// Bonus for pawn chains: a pawn gets +2 if defended by a pawn on
	// bottom-left or bottom-right (white attack geometry in H1=LSB mapping).
	const fileA uint64 = 0x0101010101010101
	const fileH uint64 = 0x8080808080808080
	defendedPawns := pawnBitboard & (((pawnBitboard << 7) &^ fileH) | ((pawnBitboard << 9) &^ fileA))
	score += bits.OnesCount64(defendedPawns) * 2

	return score
}
