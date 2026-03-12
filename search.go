package main

import (
	"fmt"
	"math"

	"github.com/corentings/chess"
)

// nodesVisited counts total nodes evaluated during a search — useful for benchmarking speed.
var nodesVisited int = 0

// ttEntry stores a cached position evaluation together with the search depth it was computed at.
// Entries are only returned when storedDepth >= requestedDepth, ensuring a shallow result
// is never substituted for a deeper one (which caused incorrect moves in earlier versions).
type ttEntry struct {
	score float64
	depth uint8
}

// transpositionTable caches position evaluations by Zobrist hash.
// Persists across iterative deepening iterations — depth validation keeps entries correct.
var transpositionTable = make(map[[16]byte]ttEntry)

// killerMoveTable stores "killer moves" per depth — moves that caused a beta cutoff
// in a sibling node and are likely to be strong in the current node too.
var killerMoveTable = make(map[uint8][]Move)

// lastBestMoves is a set of moves found best at previous iterative deepening depths.
// Stored by square pair (Move struct) for O(1) lookup with no string allocation.
var lastBestMoves = make(map[Move]bool)

// minimax implements the Minimax algorithm with Alpha-Beta Pruning.
// It recursively explores the game tree to depth `depth`, alternating between
// the maximizer (White) and minimizer (Black) at each level.
//
// Alpha-Beta pruning skips branches that cannot possibly affect the final result:
//   - alpha: the best score the maximizer is guaranteed so far
//   - beta:  the best score the minimizer is guaranteed so far
//   - if beta <= alpha, the current branch is pruned (opponent won't allow this line)
//
// Also implements Late Move Reduction (LMR): moves in the second half of the sorted
// list are searched at depth-2 instead of depth-1. If they beat the current best,
// a full re-search at depth-1 is done to confirm. This saves significant time since
// later moves (after good ordering) are unlikely to be best.
func minimax(position *chess.Position, depth uint8, maximizer bool, alpha float64, beta float64, pst map[chess.Color]map[chess.PieceType][64]int) float64 {
	nodesVisited++

	// Transposition table lookup: if we've already evaluated this exact position, reuse it.
	// Transposition table lookup: only reuse a cached result if it was computed at a depth
	// at least as deep as what we currently need. A depth-1 entry is useless at depth-6.
	if entry, ok := transpositionTable[position.Hash()]; ok && entry.depth >= depth {
		return entry.score
	}

	// Terminal node: game is over (checkmate or stalemate). Evaluate and cache.
	if position.Status() != chess.NoMethod {
		eval := float64(EvaluatePos(position, pst))
		transpositionTable[position.Hash()] = ttEntry{eval, 255}
		return eval
	}

	// Leaf node: run quiescence search instead of returning the raw static eval.
	// This prevents the "horizon effect" — stopping right before a capture gives a misleading score.
	if depth == 0 {
		eval := quiescence_search(position, int(alpha), int(beta), maximizer, 1, pst)
		transpositionTable[position.Hash()] = ttEntry{float64(eval), 0}
		return float64(eval)
	}

	// Generate and sort all legal moves, best first.
	// Good move ordering is critical: the sooner we find a strong move,
	// the more branches alpha-beta can prune.
	// Pre-compute scores once, then sort both arrays together to avoid
	// redundant EvaluateMove calls during sort comparisons.
	moves := position.ValidMoves()
	moveScores := make([]int, len(moves))
	for i, m := range moves {
		moveScores[i] = EvaluateMove(m, position, depth)
	}
	// Simple selection sort keeps moves and scores in sync without extra allocation.
	for i := 0; i < len(moves); i++ {
		best := i
		for j := i + 1; j < len(moves); j++ {
			if moveScores[j] > moveScores[best] {
				best = j
			}
		}
		moves[i], moves[best] = moves[best], moves[i]
		moveScores[i], moveScores[best] = moveScores[best], moveScores[i]
	}

	if maximizer {
		bestScore := math.Inf(-1)
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			score := 0.0
			// Late Move Reduction: moves in the second half of the list are likely weaker.
			// Search them at reduced depth first; only do a full search if they look promising.
			if len(moves)/2 < i && depth >= 3 {
				score = minimax(position.Update(moves[i]), depth-2, !maximizer, alpha, beta, pst)
				if score > bestScore {
					// Promising — confirm with a full-depth search.
					score := minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
					if score > bestScore {
						bestMove = moves[i]
						bestScore = score
					}
				}
			} else {
				// Normal full-depth search for early (likely better) moves.
				score = minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
				if score > bestScore {
					bestMove = moves[i]
					bestScore = score
				}
			}
			alpha = math.Max(alpha, score)
			// Beta cutoff: minimizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				break
			}
		}
		// Record the best move as a killer for future searches at this depth.
		storeKillerMove(depth, Move{bestMove.S1(), bestMove.S2()})
		transpositionTable[position.Hash()] = ttEntry{bestScore, depth}
		return bestScore
	} else {
		bestScore := math.Inf(1)
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			score := 0.0
			// Late Move Reduction (minimizer side).
			if len(moves)/2 < i && depth >= 3 {
				score = minimax(position.Update(moves[i]), depth-2, !maximizer, alpha, beta, pst)
				if score < bestScore {
					// Promising — confirm with a full-depth search.
					score := minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
					if score < bestScore {
						bestMove = moves[i]
						bestScore = score
					}
				}
			} else {
				score = minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
				if score < bestScore {
					bestMove = moves[i]
					bestScore = score
				}
			}
			beta = math.Min(beta, score)
			// Alpha cutoff: maximizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				break
			}
		}
		storeKillerMove(depth, Move{bestMove.S1(), bestMove.S2()})
		transpositionTable[position.Hash()] = ttEntry{bestScore, depth}
		return bestScore
	}
}

// rateAllMoves searches all root-level moves at the given depth and returns the best move
// along with its score. The transposition table is NOT cleared between depth iterations;
// depth-aware entries from shallower searches remain valid and accelerate deeper ones.
// Castling gets a +200 root bonus to encourage the engine to castle when it's roughly equal.
func rateAllMoves(position *chess.Position, depth uint8, pst map[chess.Color]map[chess.PieceType][64]int, isWhite bool) (*chess.Move, float64) {
	bestMove := &chess.Move{}
	bestScore := math.Inf(-1)
	if !isWhite {
		bestScore = math.Inf(1)
	}

	moves := position.ValidMoves()

	for _, move := range moves {
		score := minimax(position.Update(move), depth-1, !isWhite, math.Inf(-1), math.Inf(1), pst)
		// Apply a root-level castling bonus — castling is good for king safety.
		if isCastlingMove(move) {
			score += 200
		}
		if (isWhite && (score > bestScore)) || (!isWhite && (score < bestScore)) {
			bestScore = score
			bestMove = move
		}
	}

	return bestMove, bestScore
}

// iterativeDeepening runs rateAllMoves repeatedly from depth 1 up to maxDepth.
// This approach has two major advantages:
//  1. A valid best move is always available (even if search is interrupted early via stopSearch).
//  2. Best moves found at shallower depths are stored in lastBestMoves and used to
//     boost move ordering at deeper depths, increasing alpha-beta cutoffs.
//
// UCI engines emit "info depth X score cp Y" lines so the GUI can track search progress.
func iterativeDeepening(position *chess.Position, maxDepth uint8, pst map[chess.Color]map[chess.PieceType][64]int, isWhite bool) {
	bestMove := &chess.Move{}
	bestScore := 0.0
	for i := 0; i < int(maxDepth); i++ {
		select {
		case <-stopSearch:
			// Search was interrupted (e.g. UCI "stop" command) — return whatever we have.
			fmt.Println("bestmove", bestMove)
			return
		default:
			bestMove, bestScore = rateAllMoves(position, uint8(i+1), pst, isWhite)
			lastBestMoves[Move{bestMove.S1(), bestMove.S2()}] = true
			fmt.Printf("info depth %d score cp %f \n", i+1, bestScore)
		}
	}
	fmt.Println("bestmove", bestMove)
	// Clean up state for the next search.
	lastBestMoves = make(map[Move]bool)
	killerMoveTable = resetKillerMoveTable(killerMoveTable)
}
