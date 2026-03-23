package main

//TRANSPOSITION TABLE CONSTANTS
// ttSize is the number of entries in the array-based transposition table.
// Must be a power of 2 so we can use bitwise AND for fast index computation.
const ttSize = 1 << 20 // 1,048,576 entries
const (
	ttBoundExact uint8 = iota
	ttBoundLower
	ttBoundUpper
)

//PST CONSTANTS
const (
	pstStart = iota
	pstMiddle
	pstEnd
)

//SEARCH CONSTANTS

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

	// Aspiration window settings for root searches.
	// aspirationMargin is the ±cp band around the previous iteration's score.
	// Searches that fail outside this band are retried with full window [minScore, maxScore].
	// Typical values are 15-50; higher values reduce re-searches but use wider windows.
	aspiratingWindowMargin = 17

	// aspirationMinDepth is the minimum depth at which aspiration windows are applied.
	// Shallower searches have less predictive value from previous iteration.
	aspirationMinDepth = 3

	// pvFollowBonus strongly prioritizes a move that the previous completed PV predicted
	// for the current position hash. This is ordering-only and does not skip other moves.
	pvFollowBonus = 1000
)

//QUIESCENCE SEARCH CONSTANTS
// deltaMargin is the safety margin for delta pruning in quiescence search.
// If the static eval plus the value of the captured piece plus this margin
// cannot reach alpha, the capture is futile and can be skipped.
const deltaMargin = 200

//PROFILING CONSTANTS
const benchmarkCalls = 1000000
