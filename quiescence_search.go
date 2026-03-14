package main

import (
	"github.com/corentings/chess"
)

// deltaMargin is the safety margin for delta pruning in quiescence search.
// If the static eval plus the value of the captured piece plus this margin
// cannot reach alpha, the capture is futile and can be skipped.
const deltaMargin = 200

// quiescence_search extends the search beyond the normal depth limit, but only for
// "loud" moves (captures and checks). This solves the horizon effect: if minimax
// stops searching right before a piece is captured, the position looks falsely stable.
//
// The key idea is "stand pat": the static evaluation of the current position serves
// as a lower bound for the maximizer. If we're already better than beta without making
// any move, the opponent wouldn't have allowed this line — prune immediately.
//
// Delta pruning is applied: captures that cannot possibly raise the score above alpha
// (even with the full value of the captured piece plus a safety margin) are skipped.
//
// The depth parameter limits the quiescence search to prevent explosion
// (positions with many forced captures could recurse very deeply otherwise).
func quiescence_search(pos *chess.Position, alpha int, beta int, maximizer bool, depth uint8, pst [3][3][7][64]int) int {
	nodesVisited++

	// Stand-pat evaluation: the score if we make no more captures ("stand pat").
	stand_eval := EvaluatePos(pos, pst)

	// Depth limit reached — return the static eval without searching further.
	if depth == 0 {
		return stand_eval
	}

	// Stand-pat beta cutoff: if the static eval already exceeds beta,
	// the opponent wouldn't allow this position — no need to search captures.
	if stand_eval >= beta {
		return stand_eval
	}

	// Stand-pat improves alpha: we can "do nothing" and already beat our previous best.
	if stand_eval > alpha {
		alpha = stand_eval
	}

	vm := pos.ValidMoves()

	if maximizer {
		max_Eval := stand_eval
		for _, move := range vm {
			// Only explore captures and checks — quiet moves are ignored.
			if move.HasTag(chess.Capture) || move.HasTag(chess.Check) {
				// Delta pruning: if even winning the captured piece can't raise us to alpha, skip.
				if move.HasTag(chess.Capture) {
					victim := pos.Board().Piece(move.S2())
					if victim.Type() != chess.NoPieceType {
						if stand_eval+pieceValues[victim.Type()]+deltaMargin < alpha {
							continue
						}
					}
				}
				newPos := pos.Update(move)
				eval := quiescence_search(newPos, alpha, beta, false, depth-1, pst)
				if eval > alpha {
					alpha = eval
				}
				if eval > max_Eval {
					max_Eval = eval
				}
				// Beta cutoff: minimizer has a better option elsewhere.
				if beta <= alpha {
					break
				}
			}
		}
		return max_Eval
	} else {
		minEval := stand_eval
		for _, move := range vm {
			// Only explore captures and checks — quiet moves are ignored.
			if move.HasTag(chess.Capture) || move.HasTag(chess.Check) {
				// Delta pruning (minimizer): if even winning the captured piece can't lower us to beta, skip.
				if move.HasTag(chess.Capture) {
					victim := pos.Board().Piece(move.S2())
					if victim.Type() != chess.NoPieceType {
						if stand_eval-pieceValues[victim.Type()]-deltaMargin > beta {
							continue
						}
					}
				}
				newPos := pos.Update(move)
				eval := quiescence_search(newPos, alpha, beta, true, depth-1, pst)
				if eval < beta {
					beta = eval
				}
				if eval < minEval {
					minEval = eval
				}
				// Alpha cutoff: maximizer has a better option elsewhere.
				if beta <= alpha {
					break
				}
			}
		}
		return minEval
	}
}
