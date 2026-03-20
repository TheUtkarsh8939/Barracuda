package main

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/corentings/chess/v2"
)

// TEMPORARY
var positionUpdateCalls int = 0

// Score bounds used as sentinel values instead of float64 math.Inf.
// Using int throughout the search avoids expensive float64 operations.
const (
	// maxScore / minScore act as +∞ and -∞ for alpha and beta initialization.
	// Values are large enough to exceed any realistic evaluation (max material ≈ 8000 cp)
	// but small enough to leave headroom for depth-dependent adjustments without overflow.
	maxScore = 999999
	minScore = -999999

	// quiescenceDepth limits how many extra plies the quiescence search extends beyond the
	// main search horizon. Each ply only considers captures and checks (not all legal moves),
	// so the branching factor is much lower than the main search. 3 provides enough depth
	// to resolve most tactical sequences (e.g. exchange chains, back-rank checks) while
	// keeping the node count manageable. Increase this for better tactical accuracy;
	// decrease it if the engine spends too long in quiescence on forcing positions.
	quiescenceDepth = 3

	// lmrMinDepth is the minimum remaining depth at which Late Move Reduction is applied.
	// At depths shallower than this the search is already cheap, so reducing further risks
	// missing important moves near the leaf. Only nodes with depth >= lmrMinDepth are reduced.
	lmrMinDepth = 4

	// lmrMoveIndex is the 0-based move index after which LMR kicks in.
	// The first lmrMoveIndex moves are always searched at full depth regardless of score,
	// because after good move ordering the best move is almost always in this group.
	// Later moves (index >= lmrMoveIndex) with a low ordering score are reduced to depth-2
	// first, then confirmed at full depth only if they beat the current best.
	lmrMoveIndex = 4

	// Null-move pruning settings. At sufficient depth, we search a "pass" move with
	// reduced depth; if it still fails high/low, we can prune this node.
	nullMoveMinDepth  = 3
	nullMoveReduction = 2
)

// lastBestMoves is a set of moves found best at previous iterative deepening depths.
// Stored by square pair (Move struct) for O(1) lookup with no string allocation.
var lastBestMoves = make(map[Move]bool)

// moveWithScore pairs a move with its evaluation score for efficient sorting.
type moveWithScore struct {
	move  *chess.Move
	score int
}

// pickNextBestMove does one selection-sort step in place.
// This lets alpha-beta stop early without paying the full sort.Slice cost first.
func pickNextBestMove(moveList []moveWithScore, start int) moveWithScore {
	bestIdx := start
	bestScore := moveList[start].score
	for j := start + 1; j < len(moveList); j++ {
		if moveList[j].score > bestScore {
			bestScore = moveList[j].score
			bestIdx = j
		}
	}
	moveList[start], moveList[bestIdx] = moveList[bestIdx], moveList[start]
	return moveList[start]
}

// hasNonPawnMaterial helps avoid null-move pruning in pawn-only endgames (zugzwang-prone).
func hasNonPawnMaterial(position *chess.Position) bool {
	board := position.Board()
	for sq := 0; sq < 64; sq++ {
		p := board.Piece(chess.Square(sq))
		if p == chess.NoPiece {
			continue
		}
		pt := p.Type()
		if pt != chess.King && pt != chess.Pawn {
			return true
		}
	}
	return false
}

// hashToUint64 converts a [16]byte Zobrist hash to uint64 for TT indexing.
func hashToUint64(h [16]byte) uint64 {
	return binary.LittleEndian.Uint64(h[:8])
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
func minimax(position *chess.Position, depth uint8, maximizer bool, alpha int, beta int, posHash uint64, pst *PST, allowNull bool) int {
	nodesVisited++

	alphaOrig := alpha
	betaOrig := beta

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
		// eval := 0 //Temporarily disabled to calculate minimax overhead without eval time included.
		// eval := EvaluatePos(position, pst) //Temporarily disabled to calculate minimax overhead without eval time included.

		ttStore(posHash, eval, 255, ttBoundExact)
		return eval
	}

	// Leaf node: run quiescence search instead of returning the raw static eval.
	// This prevents the "horizon effect" — stopping right before a capture gives a misleading score.
	if depth == 0 {
		leafNodesVisited++

		eval := quiescence_search(position, alpha, beta, maximizer, quiescenceDepth, pst)
		// eval := EvaluatePos(position, pst) //Temporarily disabled to calculate minimax overhead without eval time included.
		// eval := 0
		ttStore(posHash, eval, 0, ttBoundExact)
		return eval
	}

	// Null-move pruning: if even after giving the opponent a free move the position is still
	// good enough to fail high/low, this node can often be pruned safely.
	if allowNull && depth >= nullMoveMinDepth && hasNonPawnMaterial(position) {
		nullPos := position.Update(nil)
		nullHash := fastPosHash(nullPos)
		if maximizer {
			nullScore := minimax(nullPos, depth-1-nullMoveReduction, false, beta-1, beta, nullHash, pst, false)
			if nullScore >= beta {
				return nullScore
			}
		} else {
			nullScore := minimax(nullPos, depth-1-nullMoveReduction, true, alpha, alpha+1, nullHash, pst, false)
			if nullScore <= alpha {
				return nullScore
			}
		}
	}

	// Generate and score all legal moves for sorting.
	// Good move ordering is critical: the sooner we find a strong move,
	// the more branches alpha-beta can prune.
	movesRaw := position.ValidMoves()
	moveList := make([]moveWithScore, len(movesRaw))
	for i, moveObj := range movesRaw {
		m := &moveObj
		moveList[i] = moveWithScore{
			move:  m,
			score: EvaluateMove(m, position, depth),
		}
	}

	if maximizer {
		bestScore := minScore
		bestMove := &chess.Move{}
		for i := 0; i < len(moveList); i++ {
			var score int
			picked := pickNextBestMove(moveList, i)
			currentMove := picked.move
			positionUpdateCalls++
			child := position.Update(currentMove)
			childHash := fastChildHash(position, child, currentMove, posHash)
			// Late Move Reduction: moves beyond the first few are likely weaker after good ordering.
			// Search them at reduced depth first; only do a full search if they look promising.
			if i >= lmrMoveIndex && depth >= lmrMinDepth && picked.score < 50 {
				score = minimax(child, depth-2, !maximizer, alpha, beta, childHash, pst, true)
				if score > alpha {
					// Promising — confirm with a full-depth search.
					score = minimax(child, depth-1, !maximizer, alpha, beta, childHash, pst, true)
				}
			} else {
				// Normal full-depth search for early (likely better) moves.
				score = minimax(child, depth-1, !maximizer, alpha, beta, childHash, pst, true)
			}
			if score > bestScore {
				bestMove = currentMove
				bestScore = score
			}
			if bestScore > alpha {
				alpha = bestScore
			}
			// Beta cutoff: minimizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				if !currentMove.HasTag(chess.Capture) && currentMove.Promo() == chess.NoPieceType {
					storeKillerMove(depth, Move{currentMove.S1(), currentMove.S2()})
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
		for i := 0; i < len(moveList); i++ {
			var score int
			picked := pickNextBestMove(moveList, i)
			currentMove := picked.move
			positionUpdateCalls++
			child := position.Update(currentMove)
			childHash := fastChildHash(position, child, currentMove, posHash)
			// Late Move Reduction (minimizer side).
			if i >= lmrMoveIndex && depth >= lmrMinDepth && picked.score < 50 {
				score = minimax(child, depth-2, !maximizer, alpha, beta, childHash, pst, true)
				if score < beta {
					// Promising — confirm with a full-depth search.
					score = minimax(child, depth-1, !maximizer, alpha, beta, childHash, pst, true)
				}
			} else {
				score = minimax(child, depth-1, !maximizer, alpha, beta, childHash, pst, true)
			}
			if score < bestScore {
				bestMove = currentMove
				bestScore = score
			}
			if bestScore < beta {
				beta = bestScore
			}
			// Alpha cutoff: maximizer already has a better option elsewhere, prune this branch.
			if beta <= alpha {
				if !currentMove.HasTag(chess.Capture) && currentMove.Promo() == chess.NoPieceType {
					storeKillerMove(depth, Move{currentMove.S1(), currentMove.S2()})
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
func rateAllMoves(position *chess.Position, depth uint8, pst *PST, isWhite bool) (*chess.Move, int) {
	rootHash := fastPosHash(position)
	bestMove := &chess.Move{}
	bestScore := minScore
	alpha := minScore
	beta := maxScore

	if !isWhite {
		bestScore = maxScore
	}

	moves := position.ValidMoves()

	for _, moveObj := range moves {
		//Getting the move pointer
		move := &moveObj
		child := position.Update(move)
		positionUpdateCalls++
		childHash := fastChildHash(position, child, move, rootHash)
		score := minimax(child, depth-1, !isWhite, alpha, beta, childHash, pst, true)
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
func iterativeDeepening(position *chess.Position, maxDepth uint8, pst *PST, isWhite bool) {
	nodesVisited = 0
	leafNodesVisited = 0
	quiescenceNodesVisited = 0
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
	// Clean up state for the next search.
	lastBestMoves = make(map[Move]bool)
	clearKillerTable()
	clearPV()
}
