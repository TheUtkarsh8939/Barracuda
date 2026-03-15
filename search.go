package main

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/corentings/chess"
)

// Score bounds used as sentinel values instead of float64 math.Inf.
// Using int throughout the search avoids expensive float64 operations.
const (
	maxScore        = 999999
	minScore        = -999999
	quiescenceDepth = 3
	lmrMinDepth     = 4
	lmrMoveIndex    = 4
)

// nodesVisited counts total nodes evaluated during a search — useful for benchmarking speed.
var nodesVisited int = 0

// ttSize is the number of entries in the array-based transposition table.
// Must be a power of 2 so we can use bitwise AND for fast index computation.
const ttSize = 1 << 20 // ~1M entries

// ttMask is used to compute the TT index via hash & ttMask (equivalent to hash % ttSize).
const ttMask = ttSize - 1

// ttEntry stores a cached position evaluation together with the search depth it was computed at.
// Entries are only returned when storedDepth >= requestedDepth, ensuring a shallow result
// is never substituted for a deeper one (which caused incorrect moves in earlier versions).
// The hashKey field stores the upper bits of the Zobrist hash for collision detection.
type ttEntry struct {
	hashKey uint64
	score   int
	depth   uint8
	bound   uint8
}

const (
	ttBoundExact uint8 = iota
	ttBoundLower
	ttBoundUpper
)

// transpositionTable caches position evaluations indexed by Zobrist hash.
// Array-based with modulo indexing for ~10x faster lookups than Go maps.
// Persists across iterative deepening iterations — depth validation keeps entries correct.
var transpositionTable [ttSize]ttEntry

// lastBestMoves is a set of moves found best at previous iterative deepening depths.
// Stored by square pair (Move struct) for O(1) lookup with no string allocation.
var lastBestMoves = make(map[Move]bool)

// pvEntry stores the best move found for a position at a given depth.
// This allows reconstructing the principal variation (predicted best move chain)
// from the root down to the searched leaf.
type pvEntry struct {
	hashKey uint64
	depth   uint8
	moveUCI string
}

// pvTable is an array-based principal variation table indexed exactly like TT.
var pvTable [ttSize]pvEntry

// lastPrincipalVariation stores the latest full best line from root to leaf.
var lastPrincipalVariation []string

// hashToUint64 converts a [16]byte Zobrist hash to uint64 for TT indexing.
func hashToUint64(h [16]byte) uint64 {
	return binary.LittleEndian.Uint64(h[:8])
}

// ttLookup probes the transposition table for a cached entry.
// Exact scores can be returned immediately. Bound entries are used to tighten
// the alpha-beta window and can trigger an immediate cutoff if the window closes.
func ttLookup(h uint64, depth uint8, alpha int, beta int) (int, int, int, bool) {
	idx := h & ttMask
	entry := &transpositionTable[idx]
	if entry.hashKey == h && entry.depth >= depth {
		switch entry.bound {
		case ttBoundExact:
			return entry.score, alpha, beta, true
		case ttBoundLower:
			if entry.score > alpha {
				alpha = entry.score
			}
		case ttBoundUpper:
			if entry.score < beta {
				beta = entry.score
			}
		}
		if alpha >= beta {
			return entry.score, alpha, beta, true
		}
	}
	return 0, alpha, beta, false
}

// ttStore saves an evaluation in the transposition table.
func ttStore(h uint64, score int, depth uint8, bound uint8) {
	idx := h & ttMask
	transpositionTable[idx] = ttEntry{hashKey: h, score: score, depth: depth, bound: bound}
}

// clearTT resets the transposition table for a new search.
func clearTT() {
	transpositionTable = [ttSize]ttEntry{}
}

// pvLookup returns the cached best move for this position if available at sufficient depth.
func pvLookup(h uint64, depth uint8) (string, bool) {
	idx := h & ttMask
	entry := &pvTable[idx]
	if entry.hashKey == h && entry.depth >= depth && entry.moveUCI != "" {
		return entry.moveUCI, true
	}
	return "", false
}

// pvStore saves the best move found at this node for PV reconstruction.
func pvStore(h uint64, depth uint8, move *chess.Move) {
	if move == nil {
		return
	}
	idx := h & ttMask
	pvTable[idx] = pvEntry{hashKey: h, depth: depth, moveUCI: fmt.Sprint(move)}
}

// clearPV resets principal variation state for a new search.
func clearPV() {
	pvTable = [ttSize]pvEntry{}
	lastPrincipalVariation = nil
}

// buildPVLine reconstructs the predicted best line from root at the given depth.
func buildPVLine(position *chess.Position, depth uint8) []string {
	line := make([]string, 0, depth)
	current := position

	for ply := uint8(0); ply < depth; ply++ {
		remaining := depth - ply
		moveUCI, ok := pvLookup(hashToUint64(current.Hash()), remaining)
		if !ok {
			break
		}

		line = append(line, moveUCI)
		move, err := chess.Notation.Decode(chess.UCINotation{}, current, moveUCI)
		if err != nil {
			break
		}

		current = current.Update(move)
		if current.Status() != chess.NoMethod {
			break
		}
	}

	return line
}

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
func minimax(position *chess.Position, depth uint8, maximizer bool, alpha int, beta int, pst [3][3][7][64]int) int {
	nodesVisited++
	alphaOrig := alpha
	betaOrig := beta

	// Compute hash once and reuse for both TT lookup and store.
	posHash := hashToUint64(position.Hash())

	// Transposition table lookup: only reuse a cached result if it was computed at a depth
	// at least as deep as what we currently need. A depth-1 entry is useless at depth-6.
	if score, newAlpha, newBeta, ok := ttLookup(posHash, depth, alpha, beta); ok {
		return score
	} else {
		alpha = newAlpha
		beta = newBeta
	}

	// Terminal node: game is over (checkmate or stalemate). Evaluate and cache.
	if position.Status() != chess.NoMethod {
		eval := quiescence_search(position, alpha, beta, maximizer, quiescenceDepth, pst)
		ttStore(posHash, eval, 255, ttBoundExact)
		return eval
	}

	// Leaf node: run quiescence search instead of returning the raw static eval.
	// This prevents the "horizon effect" — stopping right before a capture gives a misleading score.
	if depth == 0 {
		eval := quiescence_search(position, alpha, beta, maximizer, quiescenceDepth, pst)
		ttStore(posHash, eval, 0, ttBoundExact)
		return eval
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
		bestScore := minScore
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			var score int
			child := position.Update(moves[i])
			// Late Move Reduction: moves beyond the first few are likely weaker after good ordering.
			// Search them at reduced depth first; only do a full search if they look promising.
			if i >= lmrMoveIndex && depth >= lmrMinDepth && moveScores[i] < 50 {
				score = minimax(child, depth-2, !maximizer, alpha, beta, pst)
				if score > alpha {
					// Promising — confirm with a full-depth search.
					score = minimax(child, depth-1, !maximizer, alpha, beta, pst)
				}
			} else {
				// Normal full-depth search for early (likely better) moves.
				score = minimax(child, depth-1, !maximizer, alpha, beta, pst)
			}
			if score > bestScore {
				bestMove = moves[i]
				bestScore = score
			}
			if bestScore > alpha {
				alpha = bestScore
			}
			// Beta cutoff: minimizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				if !moves[i].HasTag(chess.Capture) && moves[i].Promo() == chess.NoPieceType {
					storeKillerMove(depth, Move{moves[i].S1(), moves[i].S2()})
				}
				break
			}
		}
		// Record the best move as a killer for future searches at this depth.
		pvStore(posHash, depth, bestMove)
		bound := ttBoundExact
		if bestScore <= alphaOrig {
			bound = ttBoundUpper
		} else if bestScore >= betaOrig {
			bound = ttBoundLower
		}
		ttStore(posHash, bestScore, depth, bound)
		return bestScore
	} else {
		bestScore := maxScore
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			var score int
			child := position.Update(moves[i])
			// Late Move Reduction (minimizer side).
			if i >= lmrMoveIndex && depth >= lmrMinDepth && moveScores[i] < 50 {
				score = minimax(child, depth-2, !maximizer, alpha, beta, pst)
				if score < beta {
					// Promising — confirm with a full-depth search.
					score = minimax(child, depth-1, !maximizer, alpha, beta, pst)
				}
			} else {
				score = minimax(child, depth-1, !maximizer, alpha, beta, pst)
			}
			if score < bestScore {
				bestMove = moves[i]
				bestScore = score
			}
			if bestScore < beta {
				beta = bestScore
			}
			// Alpha cutoff: maximizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				if !moves[i].HasTag(chess.Capture) && moves[i].Promo() == chess.NoPieceType {
					storeKillerMove(depth, Move{moves[i].S1(), moves[i].S2()})
				}
				break
			}
		}
		pvStore(posHash, depth, bestMove)
		bound := ttBoundExact
		if bestScore <= alphaOrig {
			bound = ttBoundUpper
		} else if bestScore >= betaOrig {
			bound = ttBoundLower
		}
		ttStore(posHash, bestScore, depth, bound)
		return bestScore
	}
}

// rateAllMoves searches all root-level moves at the given depth and returns the best move
// along with its score. The transposition table is NOT cleared between depth iterations;
// depth-aware entries from shallower searches remain valid and accelerate deeper ones.
// Castling gets a +200 root bonus to encourage the engine to castle when it's roughly equal.
// Uses alpha-beta window at root level to prune moves that can't improve on the best found so far.
func rateAllMoves(position *chess.Position, depth uint8, pst [3][3][7][64]int, isWhite bool) (*chess.Move, int) {
	rootHash := hashToUint64(position.Hash())
	bestMove := &chess.Move{}
	bestScore := minScore
	alpha := minScore
	beta := maxScore

	if !isWhite {
		bestScore = maxScore
	}

	moves := position.ValidMoves()

	for _, move := range moves {
		score := minimax(position.Update(move), depth-1, !isWhite, alpha, beta, pst)
		// Apply a root-level castling bonus — castling is good for king safety.
		if isCastlingMove(move) {
			score += 200
		}
		if isWhite {
			if score > bestScore {
				bestScore = score
				bestMove = move
			}
			if score > alpha {
				alpha = score
			}
		} else {
			if score < bestScore {
				bestScore = score
				bestMove = move
			}
			if score < beta {
				beta = score
			}
		}
	}

	pvStore(rootHash, depth, bestMove)

	return bestMove, bestScore
}

// iterativeDeepening runs rateAllMoves repeatedly from depth 1 up to maxDepth.
// This approach has two major advantages:
//  1. A valid best move is always available (even if search is interrupted early via stopSearch).
//  2. Best moves found at shallower depths are stored in lastBestMoves and used to
//     boost move ordering at deeper depths, increasing alpha-beta cutoffs.
//
// UCI engines emit "info depth X score cp Y" lines so the GUI can track search progress.
func iterativeDeepening(position *chess.Position, maxDepth uint8, pst [3][3][7][64]int, isWhite bool) {
	lastBestMoves = make(map[Move]bool)
	clearKillerTable()
	transpositionTable = [ttSize]ttEntry{}
	clearPV()
	bestMove := &chess.Move{}
	bestScore := 0
	for i := 0; i < int(maxDepth); i++ {
		select {
		case <-stopSearch:
			// Search was interrupted (e.g. UCI "stop" command) — return whatever we have.
			if len(lastPrincipalVariation) > 0 {
				fmt.Printf("info pv %s\n", strings.Join(lastPrincipalVariation, " "))
			}
			fmt.Println("bestmove", bestMove)
			return
		default:
			depth := uint8(i + 1)
			bestMove, bestScore = rateAllMoves(position, depth, pst, isWhite)
			lastPrincipalVariation = buildPVLine(position, depth)
			lastBestMoves[Move{bestMove.S1(), bestMove.S2()}] = true
			if len(lastPrincipalVariation) > 0 {
				fmt.Printf("info depth %d score cp %d pv %s\n", i+1, bestScore, strings.Join(lastPrincipalVariation, " "))
			} else {
				fmt.Printf("info depth %d score cp %d \n", i+1, bestScore)
			}
		}
	}
	if len(lastPrincipalVariation) > 0 {
		fmt.Printf("info pv %s\n", strings.Join(lastPrincipalVariation, " "))
	}
	fmt.Println("bestmove", bestMove)
	fmt.Println("Nodes Searched: " + fmt.Sprintf("%d", nodesVisited))
	nodesVisited = 0
	// Clean up state for the next search.
	lastBestMoves = make(map[Move]bool)
	clearKillerTable()
	clearPV()
}
