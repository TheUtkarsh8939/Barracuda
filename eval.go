package main

import (
	"math"
	"slices"

	"github.com/corentings/chess"
)

// Piece values (material advantage)
var pieceValues = map[chess.PieceType]int{
	chess.King:   100000,
	chess.Queen:  900,
	chess.Rook:   500,
	chess.Bishop: 300,
	chess.Knight: 300,
	chess.Pawn:   100,
}

type kingLoc struct {
	x int
	y int
}

func EvaluatePos(position *chess.Position, pst map[chess.Color]map[chess.PieceType][64]int) int {
	chessMap := position.Board().SquareMap()
	score := 0
	blackKing := kingLoc{0, 0}
	whiteKing := kingLoc{0, 0}
	totalMaterial := -200000 //Starts with -200000 to cancel the kings
	for k, v := range chessMap {
		pieceColor := v.Color()
		pieceType := v.Type()
		location := uint8(k.Rank())*8 + uint8(k.File())
		positionalAdv := pst[pieceColor][pieceType][location]
		material := pieceValues[pieceType]
		totalMaterial += material
		sc := positionalAdv + material
		if pieceColor == chess.Black {
			sc *= -1
		}
		if pieceType == chess.King && pieceColor == chess.White {
			whiteKing = kingLoc{int(k.File()), int(k.Rank())}
		} else if pieceType == chess.King && pieceColor == chess.Black {
			blackKing = kingLoc{int(k.File()), int(k.Rank())}
		}
		score += sc
	}
	blacksDistanceFromCenter := math.Abs(4.5-float64(blackKing.x)) + math.Abs(4.5-float64(blackKing.y))
	whitesDistanceFromCenter := math.Abs(4.5-float64(whiteKing.x)) + math.Abs(4.5-float64(whiteKing.y))
	endGameIndex := 78 - totalMaterial
	smartEndgameFactor := math.Max((float64(endGameIndex)/10)-5.8, 0)
	smartEndgameScore := (-whitesDistanceFromCenter + blacksDistanceFromCenter) * smartEndgameFactor * 50
	if position.CastleRights().CanCastle(chess.White, chess.KingSide) {
		score += 50
	}
	if position.CastleRights().CanCastle(chess.White, chess.QueenSide) {
		score += 40
	}
	if position.CastleRights().CanCastle(chess.Black, chess.KingSide) {
		score -= 50
	}
	if position.CastleRights().CanCastle(chess.Black, chess.QueenSide) {
		score -= 40
	}
	return score + int(smartEndgameScore)
}
func EvaluateMove(move *chess.Move, position *chess.Position, depth uint8) int {
	pieceValues := map[chess.PieceType]int{
		chess.Pawn:   100,
		chess.Knight: 300,
		chess.Bishop: 300,
		chess.Rook:   500,
		chess.Queen:  900,
		chess.King:   100000, // King value is arbitrarily high
	}

	score := 0
	//Prioritizes Last Best Moves from Previous searches (Iterative Deepening Integration)
	if slices.Contains(lastBestMoves, move.String()) {
		score += 700
	}
	// 1. **MVV-LVA Heuristic** (Most Valuable Victim - Least Valuable Attacker)
	if move.HasTag(chess.Capture) {
		victim := position.Board().Piece(move.S2())   // Get captured piece
		attacker := position.Board().Piece(move.S1()) // Get attacking piece
		if victim.Type() != chess.NoPieceType {
			toSet := pieceValues[victim.Type()] - pieceValues[attacker.Type()]
			if toSet < 1 {
				toSet = 30
			}
			score += toSet // MVV-LVA
		}
	}

	// 2. **Promotions** (Promoting to Queen is best)
	if move.Promo() == chess.Queen {
		score += 900 // Strongly prefer promotions to Queen
	} else if move.Promo() == chess.Rook {
		score += 500
	} else if move.Promo() == chess.Bishop || move.Promo() == chess.Knight {
		score += 300
	}

	toCheckForMate := position.Update(move)
	if toCheckForMate.Status() == chess.Checkmate {
		score += 100000 // Prioritize checkmates
	} else if move.HasTag(chess.Check) {
		score += 50 // Favor checks

	}
	//Prioritizes Castling
	if isCastlingMove(move) {
		score += 40
	}
	//Prioritizes Killer Moves
	if len(killerMoveTable[depth]) > 1 && (killerMoveTable[depth][0] == Move{move.S1(), move.S2()} || killerMoveTable[depth][1] == Move{move.S1(), move.S2()}) {
		score += 70
	}

	return score

}
