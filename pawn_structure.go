package main

import (
	"math/bits"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

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

	//Bonus for passed pawns: +50 per passed pawn.
	return score
}

// PassedPawnsMask returns a bitboard containing only the passed pawns of the
// side represented by color.
func PassedPawnsMask(pawnBitboard uint64, enemyPawnBitboard uint64, color chess.Color) uint64 {
	if pawnBitboard == 0 {
		return 0
	}

	var masks *[64]uint64
	switch color {
	case chess.White:
		masks = &PassedPawnMaskWhite
	case chess.Black:
		masks = &PassedPawnMaskBlack
	default:
		return 0
	}

	if enemyPawnBitboard == 0 {
		return pawnBitboard
	}

	var passed uint64
	for pawns := pawnBitboard; pawns != 0; pawns &= pawns - 1 {
		pawn := pawns & -pawns
		sq := bits.TrailingZeros64(pawn)
		if enemyPawnBitboard&(*masks)[sq] == 0 {
			passed |= pawn
		}
	}

	return passed
}

// PassedPawnCount returns the number of passed pawns for the side represented
// by color.
func PassedPawnCount(pawnBitboard uint64, enemyPawnBitboard uint64, color chess.Color) int {
	return bits.OnesCount64(PassedPawnsMask(pawnBitboard, enemyPawnBitboard, color))
}

// PassedPawnPotentialScore returns a soft score that measures how close pawns
// are to becoming passed pawns.
//
// Per pawn scoring:
//   - 0 blockers in passed-pawn lane mask => 5 (already passed)
//   - 1 blocker => 3
//   - 2 blockers => 2
//   - 3 blockers => 1
func PassedPawnPotentialScore(pawnBitboard uint64, enemyPawnBitboard uint64, color chess.Color) int {
	if pawnBitboard == 0 {
		return 0
	}

	var masks *[64]uint64
	switch color {
	case chess.White:
		masks = &PassedPawnMaskWhite
	case chess.Black:
		masks = &PassedPawnMaskBlack
	default:
		return 0
	}

	// No enemy pawns means every pawn is already passed (score 5 each).
	if enemyPawnBitboard == 0 {
		return bits.OnesCount64(pawnBitboard) * 5
	}

	score := 0
	for pawns := pawnBitboard; pawns != 0; pawns &= pawns - 1 {
		pawn := pawns & -pawns
		sq := bits.TrailingZeros64(pawn)
		blockers := bits.OnesCount64(enemyPawnBitboard & (*masks)[sq])
		if blockers < 3 {
			score += 3 - blockers
		}
	}

	return score
}

// handlePassedPawnsWhite is kept as a compatibility wrapper.
func handlePassedPawnsWhite(pawnBitboard uint64, enemyPawnBitboard uint64) int {
	return PassedPawnCount(pawnBitboard, enemyPawnBitboard, chess.White)
}

// handlePassedPawnsBlack is a compatibility wrapper for black passed-pawn count.
func handlePassedPawnsBlack(pawnBitboard uint64, enemyPawnBitboard uint64) int {
	return PassedPawnCount(pawnBitboard, enemyPawnBitboard, chess.Black)
}
