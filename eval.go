package main

import (
	"math"

	"github.com/corentings/chess"
)

// pieceValues maps each piece type to its standard centipawn value.
// These are used for material counting and MVV-LVA move ordering.
// Scores are from White's perspective; positive = good for White.
var pieceValues = map[chess.PieceType]int{
	chess.King:   100000, // Arbitrarily large — losing the king means losing the game
	chess.Queen:  900,
	chess.Rook:   500,
	chess.Bishop: 300,
	chess.Knight: 300,
	chess.Pawn:   100,
}

// kingLoc holds the file (x) and rank (y) of a king, used for endgame centralization scoring.
type kingLoc struct {
	x int
	y int
}

// EvaluatePos returns a static evaluation of the position in centipawns from White's perspective.
// Positive = good for White, negative = good for Black.
//
// Components:
//  1. Material: sum of all piece values on the board.
//  2. Piece-Square Tables (PST): positional bonuses per piece per square.
//  3. Castling rights: bonus for retaining the right to castle (king safety indicator).
//  4. Endgame king centralization: as material drops, the winning side's king is rewarded
//     for being near the center (active king is critical in endgames).
func EvaluatePos(position *chess.Position, pst map[chess.Color]map[chess.PieceType][64]int) int {
	chessMap := position.Board().SquareMap()
	score := 0
	blackKing := kingLoc{0, 0}
	whiteKing := kingLoc{0, 0}
	// Start at -200000 to cancel out both kings' values from the material total.
	// We only want non-king material to drive the endgame detection index.
	totalMaterial := -200000
	for k, v := range chessMap {
		pieceColor := v.Color()
		pieceType := v.Type()
		// Convert square to a flat [0,63] index for PST lookup.
		location := uint8(k.Rank())*8 + uint8(k.File())
		positionalAdv := pst[pieceColor][pieceType][location]
		material := pieceValues[pieceType]
		totalMaterial += material
		sc := positionalAdv + material
		// Negate score for Black pieces since we evaluate from White's perspective.
		if pieceColor == chess.Black {
			sc *= -1
		}
		// Track king positions for endgame centralization logic below.
		if pieceType == chess.King && pieceColor == chess.White {
			whiteKing = kingLoc{int(k.File()), int(k.Rank())}
		} else if pieceType == chess.King && pieceColor == chess.Black {
			blackKing = kingLoc{int(k.File()), int(k.Rank())}
		}
		score += sc
	}

	// Endgame king centralization:
	// As material depletes, kings should move toward the center to support pawns and give checkmate.
	// endGameIndex rises as pieces come off the board; smartEndgameFactor is 0 in the middlegame
	// and increases proportionally in the endgame.
	// maxMaterial (7800) = sum of all non-king pieces in starting position:
	// 2 queens (1800) + 4 rooks (2000) + 4 bishops (1200) + 4 knights (1200) + 16 pawns (1600)
	const maxMaterial = 7800
	blacksDistanceFromCenter := math.Abs(4.5-float64(blackKing.x)) + math.Abs(4.5-float64(blackKing.y))
	whitesDistanceFromCenter := math.Abs(4.5-float64(whiteKing.x)) + math.Abs(4.5-float64(whiteKing.y))
	endGameIndex := maxMaterial - totalMaterial
	// Only activate endgame king centralization after ~4900 material is traded (halfway through the game).
	// Divide by 100 to balance the weight against other evaluation components.
	smartEndgameFactor := math.Max((float64(endGameIndex)-4900)/100, 0)
	// Reward White if Black's king is far from center and White's is close (and vice versa).
	smartEndgameScore := (-whitesDistanceFromCenter + blacksDistanceFromCenter) * smartEndgameFactor * 50

	// Castling rights bonus: losing the right to castle permanently is a king safety risk.
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

// EvaluateMove scores a move for move ordering purposes — NOT for final evaluation.
// Higher scores mean the move should be searched earlier, which increases alpha-beta cutoffs.
//
// Ordering priorities (highest to lowest):
//  1. Iterative deepening history (+700): best moves from previous shallower searches
//  2. Promotions (+300–900): based on promotion piece value
//  3. MVV-LVA captures: high-value captures with low-value attackers score highest
//  4. Checks (+50): moves that give check are usually strong
//  5. Castling (+40): generally a good king safety move
//  6. Killer moves (+70): moves that caused cutoffs at the same depth in sibling nodes
func EvaluateMove(move *chess.Move, position *chess.Position, depth uint8) int {
	score := 0

	// Iterative deepening history: moves that were best at shallower depths are likely good here too.
	// Map lookup by square pair — O(1) with no string allocation.
	if lastBestMoves[Move{move.S1(), move.S2()}] {
		score += 700
	}

	// MVV-LVA (Most Valuable Victim – Least Valuable Attacker):
	// Prefer captures that trade up (e.g. pawn takes queen) over trades that lose material.
	// If the trade is losing (negative), we still assign a small +30 so captures are tried before quiet moves.
	if move.HasTag(chess.Capture) {
		victim := position.Board().Piece(move.S2())   // Piece being captured
		attacker := position.Board().Piece(move.S1()) // Piece doing the capturing
		if victim.Type() != chess.NoPieceType {
			toSet := pieceValues[victim.Type()] - pieceValues[attacker.Type()]
			if toSet < 1 {
				toSet = 30 // Floor: even bad captures are searched before quiet moves
			}
			score += toSet
		}
	}

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
		score += 50
	}

	// Castling is generally positive for king safety.
	if isCastlingMove(move) {
		score += 40
	}

	// Killer move bonus: this move caused a beta cutoff in a sibling node at this depth,
	// so it's worth trying early in the current node too.
	if len(killerMoveTable[depth]) > 1 && (killerMoveTable[depth][0] == Move{move.S1(), move.S2()} || killerMoveTable[depth][1] == Move{move.S1(), move.S2()}) {
		score += 70
	}

	return score
}
