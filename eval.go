package main

import (
	"fmt"
	"math/bits"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

// lastTotalMaterial tracks non-king material used by phase interpolation.
var lastTotalMaterial int = 0

// pieceValues stores centipawn values indexed by PieceType (int8).
// Using an array instead of a map eliminates map hashing overhead in the hot path.
// Index 0 = King, 1 = Queen, 2 = Rook, 3 = Bishop, 4 = Knight, 5 = Pawn
var pieceValues = [6]int{
	100000, // King — arbitrarily large, losing the king means losing the game
	900,    // Queen
	500,    // Rook
	300,    // Bishop
	300,    // Knight
	100,    // Pawn
}
var legacyPieceValues = [7]int{
	0,
	100000, // King — arbitrarily large, losing the king means losing the game
	900,    // Queen
	500,    // Rook
	300,    // Bishop
	300,    // Knight
	100,    // Pawn
}

// absInt returns the absolute value of an int without float64 conversion.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clampInt(x int, lo int, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// phaseWeights returns start/mid/end weights normalized to 24.
func phaseWeights(totalMaterial int) (int, int, int) {
	const maxMaterial = 7800

	startWeight := 0
	if totalMaterial > 5200 {
		startWeight = (totalMaterial - 5200) * 24 / (maxMaterial - 5200)
	}
	startWeight = clampInt(startWeight, 0, 24)

	endWeight := 0
	if totalMaterial < 2600 {
		endWeight = (2600 - totalMaterial) * 24 / 2600
	}
	endWeight = clampInt(endWeight, 0, 24)

	midWeight := 24 - startWeight - endWeight
	if midWeight < 0 {
		// Renormalize edge cases so weights always sum to 24.
		sum := startWeight + endWeight
		if sum == 0 {
			return 0, 24, 0
		}
		startWeight = startWeight * 24 / sum
		endWeight = 24 - startWeight
		midWeight = 0
	}

	return startWeight, midWeight, endWeight
}

// EvaluateBishopAndRookPair returns a bonus score for having a pair of bishops or rooks on the same file.
func EvaluateBishopAndRookPair(bb uint64) int {
	score := 0
	for i := range fileMasks {
		mask := fileMasks[i]
		masked := bb & mask
		if bits.OnesCount64(masked) > 1 {
			score += 10 // Bonus for having a pair of bishops or rooks on the same file, as they can control it together.
		}
	}
	return score
}

// EvaluatePos returns a static evaluation of the position in centipawns from White's perspective.
// Positive = good for White, negative = good for Black.
//
// Components:
//  1. Material: sum of all piece values on the board.
//  2. Piece-Square Tables (PST): positional bonuses per piece per square.
//  3. Pawn Structure: evaluates doubled, isolated, and backward pawns based on bitboard patterns.
//  4. Endgame king centralization: as material drops, the winning side's king is rewarded
//     for being near the center (active king is critical in endgames).
func EvaluatePos(position *chess.Position, pst *PST) int {
	// Primary fast evaluator: bitboard-driven material, PST, pawn structure, and king activity.
	bitboards, err := position.MarshalBinary()
	if err != nil {
		fmt.Printf("Error in bitboard retrieval %v\n", err)
	}
	wbb, bbb := bitboards[5], bitboards[11]
	score := pawnStructure(wbb)*7 - pawnStructure(bbb)*7 //Pawn structure is worth up to ±40 centipawns, so we multiply the score by 7 to scale it appropriately with material and PST scores.

	score += (EvaluateBishopAndRookPair(bitboards[2]) - EvaluateBishopAndRookPair(bitboards[8])) * 2 // Bonus for rook pairs on the same file.
	score += (EvaluateBishopAndRookPair(bitboards[3]) - EvaluateBishopAndRookPair(bitboards[9])) * 2 // Bonus for bishop pairs on the same file.

	evaluateFunctionCalls++
	var blackKingFile, blackKingRank, whiteKingFile, whiteKingRank int
	if bitboards[0] != 0 {
		wkSq := bits.TrailingZeros64(bitboards[0])
		whiteKingFile = wkSq % 8
		whiteKingRank = wkSq / 8
	}
	if bitboards[6] != 0 {
		bkSq := bits.TrailingZeros64(bitboards[6])
		blackKingFile = bkSq % 8
		blackKingRank = bkSq / 8
	}
	// Start at -200000 to cancel out both kings' values from the material total.
	// We only want non-king material to drive the endgame detection index.
	totalMaterial := -200000
	//Get Pawn Bitboard

	openingScore := 0
	middleScore := 0
	endScore := 0
	for pieceIdx := 0; pieceIdx < 6; pieceIdx++ {
		whiteBB := bitboards[pieceIdx]
		blackBB := bitboards[pieceIdx+6]
		whiteCount := bits.OnesCount64(whiteBB)
		blackCount := bits.OnesCount64(blackBB)
		material := pieceValues[pieceIdx]

		totalMaterial += material * (whiteCount + blackCount)
		score += material * (whiteCount - blackCount)

		pieceType := pieceIdx + 1
		openingScore += AddPSTViaBitboard(whiteBB, &pst[pstStart][chess.White][pieceType])
		middleScore += AddPSTViaBitboard(whiteBB, &pst[pstMiddle][chess.White][pieceType])
		endScore += AddPSTViaBitboard(whiteBB, &pst[pstEnd][chess.White][pieceType])

		openingScore -= AddPSTViaBitboard(blackBB, &pst[pstStart][chess.Black][pieceType])
		middleScore -= AddPSTViaBitboard(blackBB, &pst[pstMiddle][chess.Black][pieceType])
		endScore -= AddPSTViaBitboard(blackBB, &pst[pstEnd][chess.Black][pieceType])
	}

	// Endgame king centralization:
	// As material depletes, kings should move toward the center to support pawns and give checkmate.
	// endGameIndex rises as pieces come off the board; smartEndgameFactor is 0 in the middlegame
	// and increases proportionally in the endgame.
	// maxMaterial (7800) = sum of all non-king pieces in starting position:
	// 2 queens (1800) + 4 rooks (2000) + 4 bishops (1200) + 4 knights (1200) + 16 pawns (1600)
	const maxMaterial = 7800
	endGameIndex := maxMaterial - totalMaterial
	lastTotalMaterial = totalMaterial
	wopening, wmiddle, wend := phaseWeights(lastTotalMaterial)
	// Phase weights are normalized to 24.
	score += (openingScore*wopening + middleScore*wmiddle + endScore*wend) / 18
	// Only activate endgame king centralization after ~4900 material is traded.
	if endGameIndex > 4800 {
		// Use integer-based distance approximation (Manhattan distance from center ~4.5).
		// Multiply distances by 2 to work in half-squares and avoid float, center at (9,9).
		blackDist := absInt(9-blackKingFile*2) + absInt(9-blackKingRank*2)
		whiteDist := absInt(9-whiteKingFile*2) + absInt(9-whiteKingRank*2)
		smartEndgameFactor := (endGameIndex - 4800) // /100 * 50 => /2
		// Reward White if Black's king is far from center and White's is close (and vice versa).
		score += (-whiteDist + blackDist) * smartEndgameFactor / 4
	} else {
		// In non endgame postions reward distance from centers
		blackDist := absInt(9-blackKingFile*2) + absInt(9-blackKingRank*2)
		whiteDist := absInt(9-whiteKingFile*2) + absInt(9-whiteKingRank*2)
		score += (whiteDist - blackDist) * 4
	}
	// Soft passed-pawn pressure: rewards pawns that are close to becoming passed.
	passedPawnPotentialWeight := 20                                      //Ranges from 0-20 depending on material traded
	endgamePawnsWhite, endgamePawnsBlack := (wbb>>32)<<32, (bbb<<32)>>32 // Only consider pawns on the opponent's half of the board for passed-pawn potential, as central pawns are more likely to become passed and create threats.This also reduces noise during opening from pawns that are still far from promotion and unlikely to become passed for many moves.
	score += (PassedPawnPotentialScore(endgamePawnsWhite, bbb, chess.White) - PassedPawnPotentialScore(endgamePawnsBlack, wbb, chess.Black)) * passedPawnPotentialWeight
	score += kingNearOpenFiles(wbb, bitboards[0]) * 5
	score -= kingNearOpenFiles(bbb, bitboards[6]) * 5
	score += rookOnOpenFiles(wbb|bbb, bitboards[2]) * 3
	score -= rookOnOpenFiles(wbb|bbb, bitboards[8]) * 3

	// Castling rights bonus: losing the right to castle permanently is a king safety risk.
	if position.CanCastle(chess.White, chess.KingSide) {
		score += 50
	}
	if position.CanCastle(chess.White, chess.QueenSide) {
		score += 40
	}
	if position.CanCastle(chess.Black, chess.KingSide) {
		score -= 50
	}
	if position.CanCastle(chess.Black, chess.QueenSide) {
		score -= 40
	}
	return score
}

// !KEPT FOR LEGACY PURPOSES
// EvaluatePos returns a static evaluation of the position in centipawns from White's perspective.
// Positive = good for White, negative = good for Black.

// Components:
//  1. Material: sum of all piece values on the board.
//  2. Piece-Square Tables (PST): positional bonuses per piece per square.
//  3. Castling rights: bonus for retaining the right to castle (king safety indicator).
//  4. Endgame king centralization: as material drops, the winning side's king is rewarded
//     for being near the center (active king is critical in endgames).
// // func LegacyEvaluatePos(position *chess.Position, pst *PST) int {
// // 	// Reference evaluator kept for parity checks and benchmarking.
// // 	bbRaw, err := position.MarshalBinary()
// // 	if err != nil {
// // 		fmt.Printf("Error in bitboard retrieval %v\n", err)
// // 	}
// // 	// Extract pawn bitboards for pawn structure evaluation.
// // 	wbb, bbb := ExtractPawnBitboards(bbRaw)
// // 	score := (pawnStructure(wbb) - pawnStructure(bbb)) * 5 //Pawn structure is worth up to ±40 centipawns, so we multiply the score by 20 to scale it appropriately with material and PST scores.
// // 	if position.Status() == chess.Checkmate {
// // 		if position.Turn() == chess.White {
// // 			return -99999 // White is checkmated
// // 		} else {
// // 			return 99999 // Black is checkmated
// // 		}
// // 	}
// // 	evaluateFunctionCalls++
// // 	board := position.Board()
// // 	var blackKingFile, blackKingRank, whiteKingFile, whiteKingRank int
// // 	// Start at -200000 to cancel out both kings' values from the material total.
// // 	// We only want non-king material to drive the endgame detection index.
// // 	totalMaterial := -200000
// // 	//Get Pawn Bitboard

// // 	openingScore := 0
// // 	middleScore := 0
// // 	endScore := 0
// // 	// Iterate squares directly instead of calling SquareMap() to avoid map allocation.
// // 	for sq := 0; sq < 64; sq++ {
// // 		v := board.Piece(chess.Square(sq))
// // 		if v == chess.NoPiece {
// // 			continue
// // 		}
// // 		pieceColor := v.Color()
// // 		pieceType := v.Type()
// // 		// Square index is already flat [0,63] for PST lookup.
// // 		opening := pst[pstStart][pieceColor][pieceType][sq]
// // 		middle := pst[pstMiddle][pieceColor][pieceType][sq]
// // 		end := pst[pstEnd][pieceColor][pieceType][sq]
// // 		material := legacyPieceValues[pieceType]
// // 		totalMaterial += material
// // 		sc := material
// // 		pstSign := 1
// // 		// Negate score for Black pieces since we evaluate from White's perspective.
// // 		if pieceColor == chess.Black {
// // 			sc = -sc
// // 			pstSign = -1
// // 		}
// // 		// Track king positions for endgame centralization logic below.
// // 		if pieceType == chess.King {
// // 			if pieceColor == chess.White {
// // 				whiteKingFile = sq % 8
// // 				whiteKingRank = sq / 8
// // 			} else {
// // 				blackKingFile = sq % 8
// // 				blackKingRank = sq / 8
// // 			}
// // 		}
// // 		score += sc
// // 		openingScore += pstSign * opening
// // 		middleScore += pstSign * middle
// // 		endScore += pstSign * end
// // 	}

// // 	// Endgame king centralization:
// // 	// As material depletes, kings should move toward the center to support pawns and give checkmate.
// // 	// endGameIndex rises as pieces come off the board; smartEndgameFactor is 0 in the middlegame
// // 	// and increases proportionally in the endgame.
// // 	// maxMaterial (7800) = sum of all non-king pieces in starting position:
// // 	// 2 queens (1800) + 4 rooks (2000) + 4 bishops (1200) + 4 knights (1200) + 16 pawns (1600)
// // 	const maxMaterial = 7800
// // 	endGameIndex := maxMaterial - totalMaterial
// // 	lastTotalMaterial = totalMaterial
// // 	wopening, wmiddle, wend := phaseWeights(lastTotalMaterial)
// // 	// Phase weights are normalized to 24.
// // 	score += (openingScore*wopening + middleScore*wmiddle + endScore*wend) / 24
// // 	// Only activate endgame king centralization after ~4900 material is traded.
// // 	if endGameIndex > 4900 {
// // 		// Use integer-based distance approximation (Manhattan distance from center ~4.5).
// // 		// Multiply distances by 2 to work in half-squares and avoid float, center at (9,9).
// // 		blackDist := absInt(9-blackKingFile*2) + absInt(9-blackKingRank*2)
// // 		whiteDist := absInt(9-whiteKingFile*2) + absInt(9-whiteKingRank*2)
// // 		smartEndgameFactor := (endGameIndex - 4900) // /100 * 50 => /2
// // 		// Reward White if Black's king is far from center and White's is close (and vice versa).
// // 		score += (-whiteDist + blackDist) * smartEndgameFactor / 4
// // 	}

// // 	// // Castling rights bonus: losing the right to castle permanently is a king safety risk.
// // 	// castleRights := position.CastleRights()
// // 	// if castleRights.CanCastle(chess.White, chess.KingSide) {
// // 	// 	score += 50
// // 	// }
// // 	// if castleRights.CanCastle(chess.White, chess.QueenSide) {
// // 	// 	score += 40
// // 	// }
// // 	// if castleRights.CanCastle(chess.Black, chess.KingSide) {
// // 	// 	score -= 50
// // 	// }
// // 	// if castleRights.CanCastle(chess.Black, chess.QueenSide) {
// // 	// 	score -= 40
// // 	// }

// // 	return score
// // }
//!LEGACY CODE ENDS

// EvaluateMove scores a move for move ordering purposes — NOT for final evaluation.
// Higher scores mean the move should be searched earlier, which increases alpha-beta cutoffs.
//
// Ordering priorities (highest to lowest):
//  1. Iterative deepening history (+700): best moves from previous shallower searches
//  2. Promotions (+300–900): based on promotion piece value
//  3. MVV-LVA captures: high-value captures with low-value attackers score highest
//  4. Checks (+100): forcing moves that often reduce reply count
//  5. Castling (+150): king safety and rook activation
//  6. Killer moves (+200): quiet moves that caused cutoffs at this depth
//
// Note: PV-follow bonus is applied in search.go where the node hash is available.
func EvaluateMove(move *chess.Move, position *chess.Position, depth uint8) int {
	score := 0
	board := position.Board()

	// Iterative deepening history: moves that were best at shallower depths are likely good here too.
	// Map lookup by square pair — O(1) with no string allocation.
	if lastBestMoves[Move{move.S1(), move.S2()}] {
		score += 700
	}

	// MVV-LVA (Most Valuable Victim – Least Valuable Attacker):
	// Prefer captures that trade up (e.g. pawn takes queen) over trades that lose material.
	// If the trade is losing (negative), we still assign a small +30 so captures are tried before quiet moves.
	if move.HasTag(chess.Capture) {
		victim := board.Piece(move.S2()) // Piece being captured
		attacker := board.Piece(move.S1())
		if victim.Type() != chess.NoPieceType {
			toSet := legacyPieceValues[victim.Type()] - legacyPieceValues[attacker.Type()]
			if toSet < 1 {
				toSet = 30 // Floor: even bad captures are searched before quiet moves
			}
			score += toSet
		}
	}

	// Lightweight pawn-structure proxy for move ordering quality.
	// score += pawnMoveOrderingBonus(move, board)

	// Promotion bonuses: scored by the value of the piece promoted to.
	if move.Promo() == chess.Queen {
		score += 900
	} else if move.Promo() == chess.Rook {
		score += 500
	} else if move.Promo() == chess.Bishop || move.Promo() == chess.Knight {
		score += 300
	}

	// Check bonus: checks are usually forcing and worth exploring early.
	if move.HasTag(chess.Check) {
		score += 100
	}

	// Castling is generally positive for king safety.
	if isCastlingMove(move) {
		score += 150
	}

	// Killer move bonus: this move caused a beta cutoff in a sibling node at this depth,
	// so it's worth trying early in the current node too.
	k0, k1, kCount := getKillerMoves(depth)
	moveKey := Move{move.S1(), move.S2()}
	if kCount > 1 && (k0 == moveKey || k1 == moveKey) {
		score += 200
	}

	return score
}
