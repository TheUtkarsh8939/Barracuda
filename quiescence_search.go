package main

import (
	"math"

	"github.com/corentings/chess"
)

func quiescence_search(pos *chess.Position, alpha int, beta int, maximizer bool, depth uint8, pst map[chess.Color]map[chess.PieceType][64]int) int {
	nodesVisited++
	stand_eval := EvaluatePos(pos, pst)
	if depth == 0 {
		return stand_eval
	}
	if stand_eval >= beta {
		// beta = stand_eval
		return stand_eval
	}
	if stand_eval > alpha {
		alpha = stand_eval
	}
	vm := pos.ValidMoves()
	if maximizer {
		max_Eval := stand_eval
		for _, move := range vm {
			if move.HasTag(chess.Capture) || move.HasTag(chess.Check) {
				newPos := pos.Update(move)
				eval := quiescence_search(newPos, alpha, beta, false, depth-1, pst)
				alpha = int(math.Max(float64(alpha), float64(eval)))
				max_Eval = int(math.Max(float64(eval), float64(max_Eval)))
				if beta <= alpha {
					break
				}
			}
		}
		return max_Eval
	} else {
		minEval := stand_eval
		for _, move := range vm {
			if move.HasTag(chess.Capture) || move.HasTag(chess.Check) {
				newPos := pos.Update(move)
				eval := quiescence_search(newPos, alpha, beta, true, depth-1, pst)
				beta = int(math.Min(float64(beta), float64(eval)))
				minEval = int(math.Min(float64(eval), float64(minEval)))
				if beta <= alpha {
					break
				}
			}
		}
		return minEval
	}
}
