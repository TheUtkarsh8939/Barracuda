package main

import (
	"math/bits"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

const (
	coreCenterMask     uint64 = 0x0000001818000000
	extendedCenterMask uint64 = 0x00003C3C3C3C0000
	historyMaxScore           = 16384
)

var rankMasks = [8]uint64{
	0x00000000000000FF,
	0x000000000000FF00,
	0x0000000000FF0000,
	0x00000000FF000000,
	0x000000FF00000000,
	0x0000FF0000000000,
	0x00FF000000000000,
	0xFF00000000000000,
}

var kingFileWindowMasks = func() [8]uint64 {
	var masks [8]uint64
	for file := 0; file < 8; file++ {
		m := fileMasks[file]
		if file > 0 {
			m |= fileMasks[file-1]
		}
		if file < 7 {
			m |= fileMasks[file+1]
		}
		masks[file] = m
	}
	return masks
}()

var whiteSpaceMask = rankMasks[3] | rankMasks[4] | rankMasks[5] | rankMasks[6]
var blackSpaceMask = rankMasks[1] | rankMasks[2] | rankMasks[3] | rankMasks[4]

var passedPawnRankBonus = [8]int{0, 5, 10, 18, 30, 48, 80, 0}

var moveOrderPieceScore = [13]int{
	0,
	20000, 900, 500, 330, 320, 100,
	20000, 900, 500, 330, 320, 100,
}

// moveHistoryTable stores quiet-move history scores by side/from/to.
var moveHistoryTable [2][64][64]int16

func clearHistoryTable() {
	moveHistoryTable = [2][64][64]int16{}
}

func ageHistoryTable() {
	for side := 0; side < 2; side++ {
		for from := 0; from < 64; from++ {
			for to := 0; to < 64; to++ {
				moveHistoryTable[side][from][to] /= 2
			}
		}
	}
}

func historySideIndex(turn chess.Color) int {
	if turn == chess.Black {
		return 1
	}
	return 0
}

func historyMoveScore(turn chess.Color, move *chess.Move) int {
	idx := historySideIndex(turn)
	return int(moveHistoryTable[idx][move.S1()][move.S2()])
}

func storeHistoryCutoff(turn chess.Color, move *chess.Move, depth uint8) {
	if move == nil {
		return
	}

	idx := historySideIndex(turn)
	from := move.S1()
	to := move.S2()

	bonus := int(depth) * int(depth) * 16
	if bonus < 16 {
		bonus = 16
	}
	if bonus > historyMaxScore {
		bonus = historyMaxScore
	}

	current := int(moveHistoryTable[idx][from][to])
	current += bonus - (current*bonus)/historyMaxScore
	if current > historyMaxScore {
		current = historyMaxScore
	}

	moveHistoryTable[idx][from][to] = int16(current)
}

func moveOrderCaptureScore(board *chess.Board, move *chess.Move, turn chess.Color) int {
	if board == nil || move == nil {
		return 0
	}

	victim := board.Piece(move.S2())
	if move.HasTag(chess.EnPassant) && victim == chess.NoPiece {
		if turn == chess.White {
			victim = chess.BlackPawn
		} else {
			victim = chess.WhitePawn
		}
	}
	if victim == chess.NoPiece {
		return 0
	}

	attacker := board.Piece(move.S1())
	if attacker == chess.NoPiece {
		return 0
	}

	victimValue := moveOrderPieceScore[victim]
	attackerValue := moveOrderPieceScore[attacker]

	score := 4800 + victimValue - attackerValue/8
	if score < 4600 {
		return 4600
	}
	return score
}

func bishopPairScore(whiteBishops uint64, blackBishops uint64, endGameIndex int) int {
	bonus := 28 + endGameIndex/240
	if bonus > 52 {
		bonus = 52
	}

	score := 0
	if bits.OnesCount64(whiteBishops) >= 2 {
		score += bonus
	}
	if bits.OnesCount64(blackBishops) >= 2 {
		score -= bonus
	}
	return score
}

func pawnIslands(pawns uint64) int {
	islands := 0
	prevOccupied := false

	for file := 0; file < 8; file++ {
		occupied := (pawns & fileMasks[file]) != 0
		if occupied && !prevOccupied {
			islands++
		}
		prevOccupied = occupied
	}

	return islands
}

func pawnIslandScore(whitePawns uint64, blackPawns uint64) int {
	whiteIslands := pawnIslands(whitePawns)
	blackIslands := pawnIslands(blackPawns)

	whitePenalty := 0
	if whiteIslands > 1 {
		whitePenalty = (whiteIslands - 1) * 8
	}
	blackPenalty := 0
	if blackIslands > 1 {
		blackPenalty = (blackIslands - 1) * 8
	}

	return blackPenalty - whitePenalty
}

func connectedPassedBonus(passed uint64) int {
	if passed == 0 {
		return 0
	}
	adjacent := ((passed &^ fileMasks[0]) >> 1) | ((passed &^ fileMasks[7]) << 1)
	return bits.OnesCount64(passed&adjacent) * 6
}

func passedPawnRankScore(pawns uint64, enemyPawns uint64, color chess.Color, wend int) int {
	passed := PassedPawnsMask(pawns, enemyPawns, color)
	if passed == 0 {
		return 0
	}

	score := connectedPassedBonus(passed)
	for bb := passed; bb != 0; bb &= bb - 1 {
		pawn := bb & -bb
		sq := bits.TrailingZeros64(pawn)
		rank := sq / 8
		if color == chess.Black {
			rank = 7 - rank
		}
		score += passedPawnRankBonus[rank]
	}

	return score * (12 + wend) / 12
}

func passedPawnScore(whitePawns uint64, blackPawns uint64, wend int) int {
	score := passedPawnRankScore(whitePawns, blackPawns, chess.White, wend)
	score -= passedPawnRankScore(blackPawns, whitePawns, chess.Black, wend)
	return score
}

func kingPawnShieldCount(kingBitboard uint64, pawnBitboard uint64, color chess.Color) int {
	if kingBitboard == 0 || pawnBitboard == 0 {
		return 0
	}

	kingSq := bits.TrailingZeros64(kingBitboard)
	kingFile := kingSq % 8
	kingRank := kingSq / 8
	fileWindow := kingFileWindowMasks[kingFile]

	zone := uint64(0)
	if color == chess.White {
		if kingRank < 7 {
			zone |= rankMasks[kingRank+1] & fileWindow
		}
		if kingRank < 6 {
			zone |= rankMasks[kingRank+2] & fileWindow
		}
	} else {
		if kingRank > 0 {
			zone |= rankMasks[kingRank-1] & fileWindow
		}
		if kingRank > 1 {
			zone |= rankMasks[kingRank-2] & fileWindow
		}
	}

	return bits.OnesCount64(zone & pawnBitboard)
}

func kingShelterScore(whiteKing uint64, blackKing uint64, whitePawns uint64, blackPawns uint64, wopening int, wmiddle int) int {
	whiteShield := kingPawnShieldCount(whiteKing, whitePawns, chess.White)
	blackShield := kingPawnShieldCount(blackKing, blackPawns, chess.Black)
	unit := 5 + (wopening*2+wmiddle)/18
	return (whiteShield - blackShield) * unit
}

func centerAndSpaceScore(whitePawns uint64, blackPawns uint64, whiteKnights uint64, blackKnights uint64, whiteBishops uint64, blackBishops uint64, wopening int, wmiddle int) int {
	pawnCenter := bits.OnesCount64(whitePawns&coreCenterMask) - bits.OnesCount64(blackPawns&coreCenterMask)
	minorCenter := bits.OnesCount64((whiteKnights|whiteBishops)&extendedCenterMask) - bits.OnesCount64((blackKnights|blackBishops)&extendedCenterMask)
	whiteSpace := bits.OnesCount64(whitePawns & whiteSpaceMask)
	blackSpace := bits.OnesCount64(blackPawns & blackSpaceMask)

	score := 0
	score += pawnCenter * (8 + wopening/6)
	score += minorCenter * (3 + wmiddle/8)
	score += (whiteSpace - blackSpace) * (1 + wmiddle/12)
	return score
}

func minorDevelopmentScore(whiteKnights uint64, blackKnights uint64, whiteBishops uint64, blackBishops uint64, wopening int) int {
	if wopening == 0 {
		return 0
	}
	whiteBackRankMinors := bits.OnesCount64((whiteKnights | whiteBishops) & rankMasks[0])
	blackBackRankMinors := bits.OnesCount64((blackKnights | blackBishops) & rankMasks[7])
	unit := 2 + wopening/6
	return (blackBackRankMinors - whiteBackRankMinors) * unit
}

func rookSeventhScore(whiteRooks uint64, blackRooks uint64, whiteKing uint64, blackKing uint64, whitePawns uint64, blackPawns uint64, wmiddle int, wend int) int {
	unit := 10 + (wmiddle+wend)/4
	score := 0

	whiteOnSeventh := bits.OnesCount64(whiteRooks & rankMasks[6])
	if whiteOnSeventh > 0 && ((blackKing&rankMasks[7]) != 0 || (blackPawns&rankMasks[6]) != 0) {
		score += whiteOnSeventh * unit
	}

	blackOnSecond := bits.OnesCount64(blackRooks & rankMasks[1])
	if blackOnSecond > 0 && ((whiteKing&rankMasks[0]) != 0 || (whitePawns&rankMasks[1]) != 0) {
		score -= blackOnSecond * unit
	}

	return score
}

func openFilePressureScore(whitePawns uint64, blackPawns uint64, whiteKing uint64, blackKing uint64, whiteRooks uint64, blackRooks uint64, wopening int, wmiddle int, wend int) int {
	allPawns := whitePawns | blackPawns
	rookWeight := 2 + (wmiddle+wend)/12
	kingWeight := 2 + wopening/6

	score := 0
	score += rookOnOpenFiles(allPawns, whiteRooks) * rookWeight
	score -= rookOnOpenFiles(allPawns, blackRooks) * rookWeight
	score += kingNearOpenFiles(whitePawns, whiteKing) * kingWeight
	score -= kingNearOpenFiles(blackPawns, blackKing) * kingWeight
	return score
}

func evaluateExtendedPositionalTerms(bitboards [12]uint64, wopening int, wmiddle int, wend int, endGameIndex int) int {
	whiteKing := bitboards[0]
	whiteRooks := bitboards[2]
	whiteBishops := bitboards[3]
	whiteKnights := bitboards[4]
	whitePawns := bitboards[5]

	blackKing := bitboards[6]
	blackRooks := bitboards[8]
	blackBishops := bitboards[9]
	blackKnights := bitboards[10]
	blackPawns := bitboards[11]

	score := 0
	score += bishopPairScore(whiteBishops, blackBishops, endGameIndex)
	score += pawnIslandScore(whitePawns, blackPawns)
	score += passedPawnScore(whitePawns, blackPawns, wend)
	score += centerAndSpaceScore(whitePawns, blackPawns, whiteKnights, blackKnights, whiteBishops, blackBishops, wopening, wmiddle)
	score += rookSeventhScore(whiteRooks, blackRooks, whiteKing, blackKing, whitePawns, blackPawns, wmiddle, wend)
	score += minorDevelopmentScore(whiteKnights, blackKnights, whiteBishops, blackBishops, wopening)
	score += kingShelterScore(whiteKing, blackKing, whitePawns, blackPawns, wopening, wmiddle)
	score += openFilePressureScore(whitePawns, blackPawns, whiteKing, blackKing, whiteRooks, blackRooks, wopening, wmiddle, wend)

	// Soft passed-pawn potential grows as the game transitions toward endgame.
	passedPawnPotentialWeight := 8 + (wend*14)/24
	endgamePawnsWhite := (whitePawns >> 32) << 32
	endgamePawnsBlack := (blackPawns << 32) >> 32
	score += (PassedPawnPotentialScore(endgamePawnsWhite, blackPawns, chess.White) - PassedPawnPotentialScore(endgamePawnsBlack, whitePawns, chess.Black)) * passedPawnPotentialWeight

	return score
}
