# Barracuda — Implementation Deep Dive

> A Go chess engine written from scratch. Uses a custom high-performance bitboard library
> (`github.com/TheUtkarsh8939/bitboardChess`) for board representation and legal move generation.
> Implements a full adversarial search stack with alpha-beta pruning, iterative deepening,
> and transposition tables. Integrates ECO opening book via a compatibility bridge layer.

---

## Table of Contents

1. [Project Structure](#1-project-structure)
2. [How a Chess Engine Works — The Mental Model](#2-how-a-chess-engine-works--the-mental-model)
3. [Search — `search.go`](#3-search--searchgo)
   - 3.1 Minimax & Alpha-Beta
   - 3.2 Late Move Reduction (LMR)
   - 3.3 Transposition Table
   - 3.4 Iterative Deepening
   - 3.5 Root Search (`rateAllMoves`) & Aspiration Windows
      - 3.5.1 Late Move Reduction at Root
      - 3.5.2 Principal Variation Search (PVS)
   - 3.6 Null-Move Pruning
   - 3.7 Move Selector (`pickNextBestMove`)
4. [Quiescence Search — `quiescence_search.go`](#4-quiescence-search--quiescence_searchgo)
5. [Evaluation — `eval.go`](#5-evaluation--evalgo)
   - 5.1 Material
   - 5.2 Piece-Square Tables
   - 5.3 Phase-Blended Evaluation
   - 5.4 Endgame King Centralization
   - 5.5 Pawn Structure Helper (`doublePawns`)
   - 5.6 Bitboard-Based Evaluator Implementation
6. [Move Ordering — `eval.go` (`EvaluateMove`)](#6-move-ordering--evalgo-evaluatemove)
7. [Piece-Square Tables — `pst.go`](#7-piece-square-tables--pstgo)
8. [Killer Move Heuristic — `handler.go`](#8-killer-move-heuristic--handlergo)
9. [Position Hashing — `hashing.go`](#9-position-hashing--hashinggo)
   - 9.1 Zobrist Hashing Primer
   - 9.2 Key Tables & splitmix64
   - 9.3 fastPosHash (Full Scan)
   - 9.4 fastChildHash (Incremental)
   - 9.5 Performance Comparison
10. [Principal Variation Store — `pv_store.go`](#10-principal-variation-store--pv_storego)
11. [Utilities — `misc.go`](#11-utilities--miscgo)
12. [UCI Protocol — `main.go` & `ucihelper.go`](#12-uci-protocol--maingo--ucihelpergo)
13. [Data Flow: One Full Search Cycle](#13-data-flow-one-full-search-cycle)
14. [Performance Notes & Optimization History](#14-performance-notes--optimization-history)
    - 14.1 Runtime Modes (`MODE=1..4`)
    - 14.2 Profiling Harness (`profiling.go`)
15. [Configuration: Centralized Tuning Constants](#15-configuration-centralized-tuning-constants)
16. [Bitboard Library Architecture — `bitboardChess/`](#16-bitboard-library-architecture--bitboardchess)
    - 16.1 Overview & Design Rationale
    - 16.2 Core Data Structures
    - 16.3 Compatibility API Layer
    - 16.4 Move Generation Algorithm
    - 16.5 Board State Representation
17. [Opening Book Integration — `opening_handler.go`](#17-opening-book-integration--opening_handlergo)
    - 17.1 ECO Opening Book System
    - 17.2 Legacy Bridge Pattern
    - 17.3 UCI Move History Tracking
18. [Build Instructions](#18-build-instructions)

---

## 1. Project Structure

```
main.go               — UCI command loop + benchmark test harness
search.go             — minimax, alpha-beta, iterative deepening, LMR, move sorting
eval.go               — static position evaluation + move ordering scorer
quiescence_search.go  — quiescence search (capture/check extension, delta pruning)
pst.go                — piece-square tables (PSTs) + mirror helper
handler.go            — killer move table management
hashing.go            — Zobrist-style hashing: fastPosHash + fastChildHash (incremental)
pv_store.go           — principal variation table: pvLookup, pvStore, buildPVLine
transposition_table.go — transposition table: ttLookup, ttStore, ttEntry with bounds
misc.go               — Move struct, SearchOptions, isCastlingMove
ucihelper.go          — parseGoCmd (UCI "go" command tokenizer)
profiling.go          — microbenchmark harness (hot-function timing + CPU usage)
profiling_cpu_*.go    — per-OS process CPU time readers (Windows/!Windows)
autoSyntaxGenerator.py — helper script used to generate file bitboard masks
go.mod / go.sum       — module definitions (direct dependency: corentings/chess)
```

**Dependency:** `github.com/corentings/chess/v2` handles the board, legal move generation,
Zobrist hashing, FEN parsing, and UCI notation encoding/decoding. Barracuda sits on top
of that and provides all the AI logic.

---

## 2. How a Chess Engine Works — The Mental Model

A chess engine splits into two independent concerns:

```
SEARCH                          EVALUATION
"Which move to look at next?"   "How good is this position?"
      |                                |
      +--------> game tree node -------+
```

The **search** explores a tree where each node is a board position and each edge is a
legal move. At leaf nodes the **evaluator** assigns a score. The search backtracks those
scores using minimax logic to figure out which root move is best.

The challenge is the search tree is astronomically large (~35 legal moves per position,
~80 plies in a full game → 35^80 nodes). Everything in Barracuda is designed to make
that tractable by pruning branches that can't possibly matter.

---

## 3. Search — `search.go`

### 3.1 Minimax & Alpha-Beta

`minimax()` is the core recursive function. It alternates between a **maximizer** (White,
wants the highest score) and a **minimizer** (Black, wants the lowest score).

```
minimax(pos, depth, maximizer, α, β, pst)
```

**Alpha-Beta Pruning** is layered on top. Two bounds are threaded through every call:

| Bound | Owned by | Meaning |
|-------|----------|---------|
| `alpha` | maximizer | "I'm already guaranteed at least this much" |
| `beta`  | minimizer | "I'm already guaranteed at most this much" |

When `beta <= alpha`, the current branch is **pruned** — the opponent would never allow
this line because they already have something better. This cuts the effective branching
factor roughly in half with perfect move ordering (O(b^(d/2)) vs O(b^d)).

### 3.2 Late Move Reduction (LMR)

After moves are sorted best-first, moves beyond the first few are assumed
to be weaker. They are searched at `depth-2` instead of `depth-1`:

```go
// Constants controlling LMR behaviour (search.go):
// lmrMoveIndex = 4  — first 4 moves always get full-depth search
// lmrMinDepth  = 4  — only apply LMR when remaining depth >= 4
if i >= lmrMoveIndex && depth >= lmrMinDepth && moveScores[i] < 50 {
    score = minimax(..., depth-2, ...)       // Reduced search
    if score > alpha {                       // (or score < beta for minimizer)
        score = minimax(..., depth-1, ...)   // Full re-search if promising
    }
}
```

If a reduced-depth search looks promising (beats alpha for maximizer, or beats beta
for minimizer), a full-depth re-search confirms it. This saves significant time
since well-ordered late moves almost never turn out to be best.

LMR is only applied at `depth >= lmrMinDepth (4)` to avoid reducing already-shallow
searches where the quality difference between depth-1 and depth-2 is critical.
The `moveScores[i] < 50` guard ensures captures, checks, promotions, and killer moves
(which all score ≥ 50 after ordering) are **never** reduced — only low-priority quiet
moves are candidates for reduction.

**Previous version:** LMR was applied after `len(moves)/2` and used `bestScore` instead
of `alpha`/`beta` for the re-search trigger. The tighter threshold (after 4 moves)
combined with the score guard produces ~45% fewer nodes with equivalent or better play.

### 3.3 Transposition Table

See also **Section 9 (Hashing)** for how position hashes are computed.

```go
const ttSize = 1 << 20  // ~1M entries
const ttMask = ttSize - 1

type ttEntry struct {
    hashKey uint64   // full hash for collision detection
    score   int
    depth   uint8
    bound   uint8    // ttBoundExact | ttBoundLower | ttBoundUpper
}
var transpositionTable [ttSize]ttEntry
```

The transposition table uses an **array-based** design indexed by `hash & ttMask`
(equivalent to `hash % ttSize` but faster since `ttSize` is a power of 2). This replaced
the original `map[[16]byte]ttEntry` which had ~100ns per lookup; array lookups are ~10ns.

The hash is a 64-bit Zobrist key (see Section 9). Inside `minimax`, each child position's
hash is computed **incrementally** via `fastChildHash(parent, child, move, parentHash)`,
avoiding a full 64-square board rescan at every node.

**Bound flags** are stored with every entry so the TT can be used for window-narrowing,
not just exact-score caching:

| Bound | Meaning | Usage |
|-------|---------|-------|
| `ttBoundExact` | Score is the true minimax value | Return immediately |
| `ttBoundLower` | Score is a lower bound (beta cutoff occurred) | Tighten alpha |
| `ttBoundUpper` | Score is an upper bound (no move beat alpha) | Tighten beta |

If tightening the window causes `alpha >= beta`, the cached bound is sufficient to
trigger an immediate cutoff even without an exact match.

```go
func ttLookup(h uint64, depth uint8, alpha int, beta int) (score int, newAlpha int, newBeta int, hit bool)
```

The critical guard is `entry.depth >= depth` — a result computed at depth 1 must not
be returned when depth 6 is needed. Entries are stored with:

- Terminal positions (checkmate/stalemate): depth `255` (valid forever)
- Quiescence results: depth `0`
- Interior nodes: their actual search depth

The TT **persists across iterative deepening iterations**. Because every entry carries its
depth, shallow results from previous iterations are valid and accelerate deeper searches.

### 3.4 Iterative Deepening

```go
func iterativeDeepening(position, maxDepth, pst, isWhite)
```

Instead of jumping straight to depth N, the engine searches depth 1, then 2, then 3, ...
up to `maxDepth`. Each full depth iteration:

1. Calls `rateAllMoves` at that depth, optionally with a narrowed aspiration window.
2. Records the best move in `lastBestMoves` (a `map[Move]bool`).
3. Updates the `predictedPVByHash` map based on the newly completed PV line.
4. Emits a UCI `info depth X score cp Y` line.
5. Checks `stopSearch` channel — if the GUI sends "stop", the last complete depth's
   best move is returned immediately.

The earlier iterations are not wasted: best moves from depth N inform move ordering at
depth N+1, dramatically increasing alpha-beta cutoffs. Additionally, the score from
depth N becomes the anchor for an **aspiration window** at depth N+1.

### 3.5 Root Search (`rateAllMoves`) & Aspiration Windows

```go
func rateAllMoves(position, depth, pst, isWhite) (*chess.Move, int)
```

Loops over every legal move at the root, calls `minimax` for each, and tracks the best.
At the root level, castling moves get a **+200 bonus** applied to their returned scores
to encourage castling when positions are otherwise roughly equal.

**Aspiration Windows:** On depths ≥ 3, rather than using the full `[-∞, +∞]` window,
the root search is called with a **narrow window centered on the previous iteration's score**:

```go
const aspiratingWindowMargin = 30  // centipawns
alpha := prevScore - aspiratingWindowMargin
beta  := prevScore + aspiratingWindowMargin
score := rateAllMoves(pos, depth, pst, isWhite, alpha, beta)
if score <= alpha || score >= beta {
    // Window failed; re-search with full window
    score = rateAllMoves(pos, depth, pst, isWhite, minScore, maxScore)
}
```

With good move ordering, the narrow window rarely fails. When it does, only a single
re-search at full window is needed. This bandwidth reduction typically saves **15–25% of nodes**
compared to always using `[-∞, +∞]`.

The root search maintains **alpha-beta bounds across root moves**. As each move is evaluated,
the alpha (for White) or beta (for Black) bound tightens, allowing `minimax` to prune more
branches for later root moves.

#### 3.5.1 Late Move Reduction at Root

Late Move Reduction (LMR) is applied at the root level in `rateAllMoves` to accelerate move evaluation.
Like the interior-node LMR (Section 3.2), root-level LMR searches weaker moves (those beyond the
first few) at reduced depth first, before committing to a full-depth search.

**Root LMR strategy:**

After moves are sorted best-first by `EvaluateMove`, the root evaluates:

1. **First 4 moves (idx 0–3):** Full-depth search `depth - 1`
2. **Remaining moves (idx ≥ 4):** Cascading search:
   - Start with depth `depth - 2` (reduced by 2 plies)
   - If the reduced score improves alpha, perform a full re-search at `depth - 1`
   - Otherwise, skip the full search and move to the next root move

**Code template:**

```go
for idx := 0; idx < len(moveList); idx++ {
    picked := pickNextBestMove(moveList, idx)
    move := picked.move
    child := position.Update(move)
    childHash := fastChildHash(position, child, move, rootHash)

    var score int
    if idx == 0 {
        // Principal move: full window, full depth
        score = minimax(child, depth-1, !isWhite, alpha, beta, childHash, pst, true)
    } else {
        // Non-principal roots: null-window first, then conditionally full re-search
        if isWhite {
            score = minimax(child, depth-1, !isWhite, alpha, alpha+1, childHash, pst, true)
            // Null-window escape: re-search at full window
            if score > alpha && score < beta {
                aspirationResearches++
                score = minimax(child, depth-1, !isWhite, alpha, beta, childHash, pst, true)
            }
        } else {
            score = minimax(child, depth-1, !isWhite, beta-1, beta, childHash, pst, true)
            if score > alpha && score < beta {
                aspirationResearches++
                score = minimax(child, depth-1, !isWhite, alpha, beta, childHash, pst, true)
            }
        }
    }

    // [Update best move and bounds]
    if isWhite {
        if score > bestScore { bestScore = score; bestMove = move }
        if score > alpha { alpha = score }
    } else {
        if score < bestScore { bestScore = score; bestMove = move }
        if score < beta { beta = score }
    }
}
```

**Cost savings:**

- First move always gets full-depth treatment (likely best due to iterative deepening)
- Moves 2–4 get full depth (still high-value candidates)
- Move 5+ get fast rejection via null-window (typically ½ the nodes of full window)
- Only moves that "escape" the null window are re-searched fully
- Overall: **10–20% reduction in root move evaluation** compared to full-window-per-move approach

This is distinct from interior-node LMR (Section 3.2), which reduces depth by 2 for weak moves.
At the root, we use PVS (null-window + full re-search) instead to avoid the overhead of
double-depth reduction on the root's already-shallow children.

#### 3.5.2 Principal Variation Search (PVS)

```go
if idx == 0 {
    // Principal move: full window search
    score = minimax(pos.Update(move), depth-1, !isWhite, alpha, beta, pst)
} else {
    // Non-principal move: null-window search first (quick rejection)
    score = minimax(pos.Update(move), depth-1, !isWhite, alpha, alpha+1, pst)
    if score > alpha && score < beta {
        // Promising; re-search with full window
        score = minimax(pos.Update(move), depth-1, !isWhite, score, beta, pst)
    }
}
// ... update bestScore ...
if isWhite { alpha = max(alpha, score) }
else       { beta  = min(beta,  score) }
```

After the first move (which is almost always best due to iterative deepening history), subsequent moves
are first searched with a **zero-width window** `[alpha, alpha+1]` for White (or `[beta-1, beta]` for Black).
This quick rejection test costs minimal computation but usually confirms that the move is worse than the best
move found so far. If the result falls within the real window `[alpha, beta]`, a full re-search confirms
whether it could be a new best move.

Expected impact: **5–15% additional node reduction** when combined with aspiration windows.

**PVS Justification:**
- Most root moves after the first are provably inferior to the current best
- Null-window search quickly confirms this with ~½ the nodes of full-window search
- Full window re-search is rare (only for moves that "escape" the null window)
- Net effect: Consistent pruning of inferior root moves without expensive full searches

**Previous version:** Every root move was searched with a full `(-∞, +∞)` window,
meaning no pruning could occur for later root moves even when an excellent move was
already found. Aspiration + PVS together typically yield **25–35% total node reduction** at depths 6–10.

### 3.6 Null-Move Pruning

if depth >= nullMoveMinDepth && !position.InCheck() && allowNull {
    // Try passing (null move) and search at reduced depth
    nullScore := minimax(position, depth - nullMoveReduction - 1, !isWhite, -beta, -alpha + 1, pst)
    if -nullScore >= beta {
        // Null move failed high; position is so good for the maximizer that
        // even giving the opponent a free move doesn't help them escape.
        // Prune this branch immediately.
        return beta
    }
}
```

**Intuition:** In overwhelming positions, letting the opponent make two free moves
(depth reduction of 2–3 plies) and still losing means the position is hopeless for them.
The "null move" is a virtual pass; both sides make real moves afterward.

**Guards against pathological positions:**

- **`allowNull` recursion guard:** Prevents two consecutive null moves in the same line.
  Null-move pruning is reset at each real move.
- **`hasNonPawnMaterial`:** Disabled in endgames with only pawns, where zugzwang (where
  moving is worse than passing) is common and null-move estimations fail badly.
- **Minimum depth gate (`nullMoveMinDepth`):** Only applied at depth ≥ 3 to avoid using
  null-move estimates in shallow searches where accuracy is critical.

**Trade-off:** Null-move pruning risks missing zugzwang positions or other rare positions
where passing would be advantageous. The guards above mitigate this, but it remains a
best-effort heuristic. In practice, the ~30–40% node reduction on typical positions
far outweighs the occasional search inaccuracy in pathological endgames.

### 3.7 Move Selector (`pickNextBestMove`)

Rather than pre-sorting all moves at each node with `sort.Slice` (which is O(n log n)),
the search uses an **incremental selection approach** to allow early cutoff:

```go
func pickNextBestMove(moves []moveWithScore, start int) int
```

Performs a single selection step: finds the move with the highest score in `moves[start:]`
and swaps it to position `start`. Returns its new position. This is O(n) for a single
selection but allows the search to cutoff after just a few moves if alpha-beta pruning
succeeds:

```go
for i := 0; i < len(validMoves); i++ {
    bestIdx := pickNextBestMove(moveScores, i)
    move := validMoves[bestIdx]
    
    score := minimax(childPos, depth-1, !isWhite, alpha, beta, pst)
    // ... alpha-beta update and cutoff check ...
    
    if betaCutoff { break }  // Early exit: no need to score remaining moves
}
```

The benefits:
- **Asymptotic:** O(n) selection is faster than O(n log n) sorting when you expect early cutoff.
- **Practical:** At most nodes, only 1–3 moves are examined before beta-cutoff, so paying the
  full sort cost (which examines all N moves) is wasteful.
- **Clean integration:** Works naturally with PVS — the first move is fully searched, subsequent
  moves can fail fast on the null window.

**Trade-off:** If a node requires deep exploration of all moves (rare), the incremental selection's
O(n²) total cost degrades compared to pre-sorting's O(n log n). However, alpha-beta pruning ensures
that few nodes require evaluating all moves in practice.

---

## 4. Quiescence Search — `quiescence_search.go`

```go
func quiescence_search(pos, alpha, beta, maximizer, depth, pst) int
```

A plain `minimax` that stops at depth 0 can misread a position badly — for example,
thinking a position is good right before an opponent captures a queen. This is the
**horizon effect**.

Quiescence search resolves this by extending the search past depth 0, but **only for
captures and checks** (not quiet moves). This continues until the position is "quiet"
(no more tactical threats), at which point the static eval is reliable.

**Stand-pat heuristic:** at each quiescence node, the static eval is taken as a baseline.

- If `stand_eval >= beta` → prune immediately (opponent wouldn't allow this)
- If `stand_eval > alpha` → update alpha (we can "do nothing" and already beat our floor)

`quiescenceDepth` is currently set to **3** in `search.go` to prevent tactical blindness
while keeping capture/check trees bounded.

The `depth` parameter limits quiescence to a fixed number of extra plies to prevent explosion
in positions with long capture chains.

### Delta Pruning

Before searching a capture in quiescence, a fast check determines whether winning the
captured piece can even raise the score above alpha:

```go
if stand_eval + pieceValues[victim.Type()] + deltaMargin < alpha {
    continue  // Skip — even the best case can't help
}
```

`deltaMargin` (200 centipawns) provides a safety buffer for positional bonuses. For the
minimizer, the symmetric check prunes captures that can't lower the score below beta:

```go
if stand_eval - pieceValues[victim.Type()] - deltaMargin > beta {
    continue  // Skip — even the best case can't help the minimizer
}
```

Delta pruning avoids the cost of `Position.Update()` and recursive evaluation for
captures that are provably futile, saving both node count and per-node overhead.

**Stand-pat early exit optimization:** `ValidMoves()` generation has been moved to *after*
the stand-pat checks in `quiescence_search`. This prevents generating captures when the
position is already so good (stand-eval ≥ beta) that it will be pruned immediately,
avoiding unnecessary allocation costs at many qsearch nodes.

**Previous version:** No delta pruning; every capture/check was searched unconditionally.
Also used `math.Max(float64(alpha), float64(eval))` for integer comparisons, which
incurred expensive int→float64 conversions at every quiescence node. These have been
replaced with simple `if` comparisons.

---

## 5. Evaluation — `eval.go`

```go
func EvaluatePos(position *chess.Position, pst *PST) int
```

Returns a score in **centipawns** from White's perspective (positive = good for White,
negative = good for Black). The primary `EvaluatePos` is a fast bitboard-based evaluator
optimized for the recursive search hot path.

### 5.1 Material

The evaluator tracks material using a hardcoded piece-value array:

```go
// Fast bitboard path: indexed by piece type only (King=0...Pawn=5)
var pieceValues = [6]int{
    100000, // King
    900,    // Queen
    500,    // Rook
    300,    // Bishop
    300,    // Knight
    100,    // Pawn
}
```

Every piece on the board contributes its value. Black pieces subtract, White add.
Extracting piece counts uses fast popcount implementations: `bits.OnesCount64(whiteBB)`.

### 5.2 Phase-Blended Evaluation

Barracuda keeps **three PST banks** (opening, middlegame, endgame) and blends them
using phase weights derived from remaining material:

```go
wopening, wmiddle, wend := phaseWeights(lastTotalMaterial)
// Phase weights are normalized to 24.
score += (openingScore*wopening + middleScore*wmiddle + endScore*wend) / 24
```

This gives smoother transitions than a single PST and prevents abrupt evaluation jumps
when material crosses a hard threshold.

### 5.3 Endgame King Centralization

In the endgame, active kings are critical for escorting pawns and delivering checkmate.
The eval rewards the side ahead for having a more central king:

```go
    const maxMaterial = 7800
    endGameIndex := maxMaterial - totalMaterial
    lastTotalMaterial = totalMaterial
    wopening, wmiddle, wend := phaseWeights(lastTotalMaterial)
    // Phase weights are normalized to 24.
    score += (openingScore*wopening + middleScore*wmiddle + endScore*wend) / 24
    // Only activate endgame king centralization after ~4900 material is traded.
    if endGameIndex > 4900 {
        // Use integer-based distance approximation (Manhattan distance from center ~4.5).
        // Multiply distances by 2 to work in half-squares and avoid float, center at (9,9).
        blackDist := absInt(9-blackKingFile*2) + absInt(9-blackKingRank*2)
        whiteDist := absInt(9-whiteKingFile*2) + absInt(9-whiteKingRank*2)
        smartEndgameFactor := (endGameIndex - 4900) // /100 * 50 => /2
        // Reward White if Black's king is far from center and White's is close (and vice versa).
        score += (-whiteDist + blackDist) * smartEndgameFactor / 4
    }
```

- Factor is **0** until ~4900 centipawns of material have been traded (halfway point)
- After that it scales linearly
- Pure integer approach: coordinates are doubled and `absInt()` replaces `math.Abs()`, eliminating all float64 operations from the evaluation hot path.

### 5.4 Pawn Structure (`pawnStructure` in `pawn_structure.go`)

`eval.go` incorporates an advanced pawn structure evaluation:

```go
score := pawnStructure(wbb)*5 - pawnStructure(bbb)*5
```

The feature applies penalties and bonuses using pure bitboard operations:
- **Doubled Pawns:** Penalizes 2+ pawns on the same file.
- **Isolated / Half-open files:** Penalizes unsupported adjacent files.
- **Pawn Chains:** Gives +2 positional bonuses for active pawn chains (pawns defended by supportive friendly pawns).
The net score difference (White - Black) is scaled via a hardcoded `*5` multiplier to balance with material and PST scores.

### 5.5 Bitboard-Based Evaluator Implementation

The fast path iterates internal `[12]uint64` bitboards directly instead of checking every square. This path:

- Extracts all 12 piece bitboards (White King, Queen, ..., Black Pawn) via `position.MarshalBinary()`.
- Uses a low-bit iteration loop to sum PST scores for each piece:
  ```go
  for bitboard != 0 {
      idx := bits.TrailingZeros64(bitboard)
      score += pst[idx ^ 7]  // file-mirror correction for PST indexing
      bitboard &= bitboard - 1  // clear lowest bit
  }
  ```
- Internally maps square indices based on bitboard representation (accounting for file mirroring).

**PST parameter and type alias:** Evaluation signatures now use `PST` type:
```go
type PST [3][3][7][64]int  // phase × color × pieceType × square
```
This reduces repetition in function signatures across `search.go`, `pv_store.go`, and other modules.

---

## 6. Move Ordering — `eval.go` (`EvaluateMove`)

```go
func EvaluateMove(move *chess.Move, position *chess.Position, depth uint8) int
```

Move ordering is critical for alpha-beta performance. Better-ordered moves cause more
cutoffs. `EvaluateMove` assigns a priority score to each move before sorting. It is
called **once per move** before the sort (cached in `moveWithScore`), not
inside the comparator. Moves are then sorted descending by score using `sort.Slice`.

| Heuristic | Bonus | Notes |
|-----------|-------|-------|
| Iterative deepening history | +700 | Best at any shallower depth — almost always good here too |
| Queen promotion | +900 | Scored by promoted piece value |
| MVV-LVA captures | variable | `pieceValues[victim] − pieceValues[attacker]`, floor 30 |
| Rook promotion | +500 | |
| Bishop / Knight promotion | +300 | |
| Principal Variation follow | +1000 | If move continues predicted best line from prior iteration |
| Killer moves | +200 | Caused beta cutoff in sibling node at same depth |
| Castling | +150 | King safety; also gets +200 root bonus in `rateAllMoves` |
| Moves that give check | +100 | Forcing; usually narrows opponent options |

*Sorting:* `pickNextBestMove()` is a O(n) lookup that finds the heighest scored move, because of the move sorting, it is usually only 2-3 move before a beta cutoff occurs, so the O(n) cost is negligible compared to the O(n log n) cost of a full sort.

**Principal Variation follow:** If a move matches the predicted continuation from the
previously completed iterative deepening depth, it receives the highest ordering priority
(+1000 bonus). This is not a line-forcing mechanism — the engine still searches all legal
moves if the opponent deviates — but it ensures that positions along stable variations are
searched first, increasing alpha-beta cutoff rates.

**MVV-LVA** (Most Valuable Victim – Least Valuable Attacker): prefers captures that trade
up (e.g. pawn takes queen = 900−100 = 800). If the net is negative (losing trade), the
score floors at 30 so all captures are still tried before quiet moves.

**Iterative deepening history:** `lastBestMoves` is a `map[Move]bool` keyed by
`{square1, square2}`. Lookup is O(1) with no string allocation (the previous
implementation used `slices.Contains` on a `[]string` which allocated on every call).

---

## 7. Piece-Square Tables — `pst.go`

Black's tables are defined explicitly as 64-value `[64]int` arrays (index 0 = a8,
index 63 = h1 in standard board orientation). White's tables are generated automatically
by `mirrorBoard()`:

```go
func mirrorBoard(pst [64]int) [64]int {
    mirrored[i] = pst[(7-rank)*8+file]  // flip rank, keep file
}
```

This ensures both sides get symmetric positional incentives.

`initPST()` returns a `[3][3][7][64]int` array indexed by
`[Phase][Color][PieceType][Square]`.

- Phase: start, middlegame, endgame
- Color: 1 = White, 2 = Black
- PieceType: 1 = King, ..., 6 = Pawn

**Previous version:** Returned `map[chess.Color]map[chess.PieceType][64]int` — a nested
map structure. Each PST lookup required two map hash operations. The array-based version
uses direct indexing (a single memory offset calculation), eliminating map hashing overhead
that was visible in CPU profiles (~17% of time spent in `aeshashbody` for map keys).

| Piece | Key incentives encoded |
|-------|----------------------|
| Pawn | Advancement toward promotion, edge penalty, center control bonus |
| Knight | Heavy edge penalty (−50 corners), strong center bonus (+20) |
| Bishop | Long diagonal control, avoid edges and corners |
| Rook | 7th rank bonus (+5/+10), open file reward |
| Queen | Slight center preference, avoid very early development |
| King | Strongly phase-dependent tables (shelter in opening, activity in endgame) |

---

## 8. Killer Move Heuristic — `handler.go`

When a move causes a **beta cutoff** (it was so good the opponent would never allow it),
it is stored as a "killer" for that depth. In sibling nodes at the same depth, killers
are tried early because they often cause cutoffs there too.

```go
const maxKillerDepth = 64

type killerEntry struct {
    moves [2]Move
    count uint8
}
var killerTable [maxKillerDepth]killerEntry
```

At most 2 killers are kept per depth, using FIFO with slot shift:
```go
entry.moves[1] = entry.moves[0]  // Demote slot 0 → slot 1
entry.moves[0] = newKiller        // New killer takes slot 0
```

`clearKillerTable()` resets the entire table after iterative deepening completes.
`getKillerMoves(depth)` returns the two killers and the count for a given depth.

**Previous version:** Used `map[uint8][]Move` which required map hashing on every lookup.
The fixed-size `[64]killerEntry` array uses direct indexing and avoids slice allocation.
`resetKillerMoveTable()` previously shifted entries by 2 depth levels between iterations;
the new version simply clears the table since the TT already preserves cross-iteration
knowledge.

---

## 9. Position Hashing — `hashing.go`

### 9.1 Zobrist Hashing Primer

A **Zobrist hash** is a 64-bit integer that represents a chess position. It is built by
XOR-ing together a unique pseudo-random key for every active feature of the position —
one key per (piece, square) pair, one for the castle rights state, one for the en passant
file, and one for the side to move:

```
hash = XOR of key[piece][sq] for every occupied square
     ^ castleKey[castleMask]
     ^ epKey[epFile]          (if en passant square exists)
     ^ turnKey                (if Black to move)
```

The critical property that makes incremental updates possible is **XOR self-inverse**:
if a piece moves from `sq1` to `sq2`, XOR-ing out `key[piece][sq1]` and XOR-ing in
`key[piece][sq2]` is equivalent to recomputing the whole hash from scratch. Any unchanged
squares contribute the same terms and cancel themselves out.

### 9.2 Key Tables & splitmix64

All keys are generated at startup by `initFastHashKeys()` using `splitmix64`, a fast
bijective hash mixer used as a deterministic PRNG. A fixed seed produces the same keys
every run, so hashes are deterministic across sessions.

```go
// Key tables (all global, initialized once in init()):
var fastHashPieceKeys   [3][7][64]uint64  // indexed by [Color][PieceType][Square]
var fastHashPieceByID  [13][64]uint64     // indexed by chess.Piece (int8): hot-path lookup
var fastHashCastleKeys [16]uint64         // 4-bit castle mask → key (16 combinations)
var fastHashEnPassantFileKeys [8]uint64   // one key per ep file (a=0 … h=7)
var fastHashTurnKey uint64                // XOR'd in when Black to move
```

`fastHashPieceByID` is a derived table that remaps `fastHashPieceKeys` by the raw
`chess.Piece` int8 value (0=NoPiece, 1=WhiteKing…12=BlackPawn). This lets the hot path
read a key with a single array index instead of calling `piece.Color()` and `piece.Type()`.

**Castle rights encoding (`fastCastleMask`):** A single pass over the castle-rights string
character by character sets 4 bits — one per right — producing a 4-bit integer (0–15)
directly usable as an index into `fastHashCastleKeys`. This replaces four `CanCastle()`
method calls.

### 9.3 fastPosHash (Full Scan)

```go
func fastPosHash(position *chess.Position) uint64
```

Iterates all 64 squares, accumulates XOR of piece keys, then XORs in the castle key, ep
key, and turn key. O(64 squares). Used only once per root call in `rateAllMoves`, and by
`buildPVLine` when following PV links.

### 9.4 fastChildHash (Incremental)

```go
func fastChildHash(parent, child *chess.Position, move *chess.Move, parentHash uint64) uint64
```

Derives the child position's hash from the parent hash by applying only the XOR deltas
caused by the move. The exact sequence:

1. **Turn key** — always toggles (both sides alternate).
2. **Castle rights** — XOR out parent's 4-bit mask, XOR in child's (rights may be lost).
3. **En passant file** — XOR out parent's ep key, XOR in child's (ep resets every ply).
4. **Source square** — XOR out the moving piece.
5. **Capture square** — XOR out any captured piece. For en passant the captured pawn is
   one rank behind the destination (not on the destination square itself).
6. **Destination square** — XOR in the placed piece (or promoted piece if a pawn promotes).
7. **Castling rook** — additionally XOR out the rook's origin and in its destination.

This is O(constant) — typically 5–10 XOR operations per move regardless of board fullness.

### 9.5 Performance Comparison

Benchmarking 1 million calls from the starting position (MODE=3, `go run .`):

| Method | Time / 1M calls | Used where |
|--------|-----------------|-----------|
| Library `position.Hash()` (md5) | ~716 ms | Previously everywhere |
| `fastPosHash` (full scan) | ~1 222 ms | Root only (once per depth) |
| `fastChildHash` (incremental) | ~245 ms | **Every minimax recursion node** |

`fastPosHash` calls `Board.Piece()` 64 times per call, making it slower than the library's
path in isolation. `fastChildHash` wins by ~3× in the actual search because it only touches
the squares affected by the move. The full-scan cost at the root is negligible compared to
the millions of recursive nodes the engine visits.

---

## 10. Principal Variation Store — `pv_store.go`

The **principal variation** (PV) is the sequence of moves the engine considers best for
both sides: `e2e4 e7e5 g1f3 b8c6 ...`. Engines emit this via `info ... pv` so the GUI
can display what the engine is "thinking."

### Why a Dedicated Table

PV entries could live in the TT, but interior-node TT traffic tends to evict them before
`buildPVLine` can traverse them. Barracuda keeps a separate `pvTable [ttSize]pvEntry`
indexed identically (by `hash & ttMask`) so PV links are never displaced by search results.

### Data Structures

```go
type pvEntry struct {
    hashKey uint64  // collision guard
    depth   uint8   // depth at which this was the best move
    moveUCI string  // e.g. "e2e4" — stored as UCI string for direct output
}

var pvTable [ttSize]pvEntry
var lastPrincipalVariation []string  // most recently built PV line
```

### API

| Function | Purpose |
|----------|---------|
| `pvStore(h, depth, move)` | Record the best move at this position+depth |
| `pvLookup(h, depth)` | Return the cached best move if depth is sufficient |
| `buildPVLine(pos, depth)` | Walk PV links from root, reconstruct move sequence |
| `clearPV()` | Reset pvTable and lastPrincipalVariation for a new search |

### buildPVLine

```go
func buildPVLine(position *chess.Position, depth uint8) []string
```

Starting from `position`, repeatedly calls `pvLookup(fastPosHash(current), remaining)`
to find the best move at each ply, applies it to get the next position, and continues
until the depth budget is exhausted or no PV entry exists. The resulting `[]string` of
UCI move tokens is stored in `lastPrincipalVariation` and emitted in the `info depth ...
pv ...` line after each iterative deepening iteration.

### PV Prediction Tracking (`predictedPVByHash`)

After each iterative deepening depth completes and `buildPVLine` constructs the latest PV,
the engine maintains a **predicted PV continuation map**:

```go
var predictedPVByHash map[uint64]Move  // position hash → next move in predicted line
```

For each position visited during the last search, if that position appears in the newly
completed PV line, the map records the move that the PV predicts from that position.
This provides a move-ordering signal for the next depth:

- When `EvaluateMove` or `minimax` evaluates a candidate move at a position in the map,
  if the move matches `predictedPVByHash[posHash]`, it receives the `pvFollowBonus`
  (+1000 in the move-ordering table, see Section 6)
- The bonus is applied at root in `rateAllMoves` and at interior nodes
- This is **not a line-forcing mechanism** — if the opponent plays a different move,
  the engine still searches all legal moves; it simply doesn't have a PV prediction
  to bias the order

The prediction map is rebuilt from each completed PV line by `buildPVLine`, so stable
principal variations naturally continue down the top of the move-ordering queue, improving
cutoff rates on consistent positions.

---

## 11. Utilities — `misc.go`

**`Move` struct:** A lightweight `{square1, square2}` pair. Used as the key type in
`killerMoveTable` and `lastBestMoves` instead of `*chess.Move` (which carries full move
metadata and is heavier to compare).

**`SearchOptions`:** In-memory representation of a parsed UCI `go` command:
```go
type SearchOptions struct {
    depth     uint8  // Fixed depth (from "go depth N")
    blackTime int    // from "btime"
    whiteTime int    // from "wtime"
    moveTime  int    // from "movetime"
    isInf     bool   // from "go infinite"
    binc      int    // from "binc"
    winc      int    // from "winc"
}
```
Note: `wtime`, `btime`, `winc`, `binc` are parsed; `movetime` exists in the struct but
is not currently parsed in `parseGoCmd`.

**`isCastlingMove`:** Detects castling by checking king origin+destination squares
(E1→G1/C1 for White, E8→G8/C8 for Black). Used both in move ordering and in the
root search (+200 bonus).

Current values in code are +150 in `EvaluateMove` and +200 root bonus in `rateAllMoves`.

**`InsertionSort`:** An O(n²) alternative to `sort.Slice` written for benchmarking.
Currently unused in production; `search.go` uses `sort.Slice` over `moveWithScore`.

**Bitboard Utilities (`ExtractPieceBitboard`, `AddPSTViaBitboard`):**

`misc.go` provides helpers for the bitboard-based evaluator:

```go
func ExtractPieceBitboard(bbRaw []byte) [12]uint64
```

Decodes all 12 piece bitboards (White King through Black Pawn) from the binary representation
returned by `Position.MarshalBinary()`. This is used by the fast `EvaluatePos` path to
extract piece locations directly instead of iterating the board.

```go
func AddPSTViaBitboard(bitboard uint64, pst *[64]int) int
```

Iterates all set bits in a bitboard using a low-bit-isolation loop:
```go
for bitboard != 0 {
    idx := bits.TrailingZeros64(bitboard)
    score += pst[idx ^ 7]  // file-mirror correction for PST indexing
    bitboard &= bitboard - 1  // clear lowest set bit
}
```

The `idx ^ 7` correction accounts for the fact that `MarshalBinary` bitboards are
file-mirrored relative to the PST square array. This index mapping was the final fix
required to achieve complete parity between the fast bitboard path and the legacy
square-iteration evaluator.

**PST Type Alias:**

```go
type PST [3][3][7][64]int  // phase × color × pieceType × square
```

Added to reduce repeated long literal array types across function signatures in `search.go`,
`pv_store.go`, `handler.go`, and evaluation code.

---

## 12. UCI Protocol — `main.go` & `ucihelper.go`

UCI (Universal Chess Interface) is the standard protocol for engine←→GUI communication.
The engine reads commands from `stdin` and writes responses to `stdout`.

### Commands handled

| Command | Response / Action |
|---------|------------------|
| `uci` | `id name Barracuda`, `id author ...`, `uciok` |
| `isready` | `readyok` |
| `position startpos [moves ...]` | Reset game, replay move list |
| `position fen <FEN> [moves ...]` | Parse FEN, replay move list |
| `go depth N` | Launch `iterativeDeepening` in a goroutine |
| `go infinite` | Search at depth 255 until `stop` |
| `stop` | Send on `stopSearch` channel → engine returns `bestmove` |
| `quit` | Send on `stopSearch`, exit process |

### `stopSearch` channel

```go
var stopSearch = make(chan bool)
```

`iterativeDeepening` checks this at the start of each depth iteration via a non-blocking
`select`. When the GUI sends `stop`, the last fully-completed depth's best move is
printed and the goroutine returns.

### `parseGoCmd` (`ucihelper.go`)

Tokenizes the `go` command string by spaces and scans for known option keys, reading
the next token as a value. Unknown tokens are silently skipped (spec-compliant).

Supported keys today: `depth`, `infinite`, `wtime`, `btime`, `winc`, `binc`.
For `go infinite`, parser sets `depth=255`.

---

## 13. Data Flow: One Full Search Cycle

```
GUI sends: "go depth 6"
        │
        ▼
parseGoCmd("go depth 6")  →  SearchOptions{depth: 6}
        │
        ▼
go iterativeDeepening(pos, 6, pst, isWhite)
        │
        ├── depth=1: rateAllMoves(pos, 1, ...)
        │     └── for each root move:
        │           minimax(childPos, 0, ...) → quiescence_search(...)
        │                                           └── EvaluatePos(...)
        │     → bestMove recorded in lastBestMoves
        │     → "info depth 1 score cp X"
        │
        ├── depth=2: rateAllMoves(pos, 2, ...)
        │     └── for each root move:
        │           minimax(childPos, 1, ...) → EvaluateMove() orders children
        │                                    → recurse ...
        │     → "info depth 2 score cp X"
        │
        ├── ... (depth 3, 4, 5)
        │
        └── depth=6: rateAllMoves(pos, 6, ...)
              └── ... deepest search, TT hits from shallower depths accelerate this
              → "info depth 6 score cp X"
              → "bestmove e2e4"
```

---

## 14. Performance Notes & Optimization History

### Round 1 — Initial optimizations

| Optimization | Impact | Where |
|---|---|---|
| Remove `position.Update()` from `EvaluateMove` (was checking for checkmate in sort) | **~5x speedup** | `eval.go` |
| Pre-compute move scores once before sort (was calling `EvaluateMove` repeatedly in comparator paths) | **~2x speedup** | `search.go` |
| Use global `pieceValues` map (was allocating a new map on every `EvaluateMove` call) | **~1.5x speedup** | `eval.go` |
| Depth-aware TT with `ttEntry{score, depth}` (was blindly reusing shallow results) | Correctness fix + moderate speed gain | `search.go` |
| TT persists across ID iterations (was wiped with `make(map...)` each depth) | Correctness + speed | `search.go` |
| `lastBestMoves` as `map[Move]bool` (was `[]string` with `slices.Contains` + string alloc) | Minor speed + no alloc | `eval.go`, `search.go` |

### Round 2 — Data structure and search overhaul

Identified via CPU profiling (`go tool pprof`). Depth 5 from startpos dropped from
~1.67s to ~0.36s (projected; measured as 371ms → 80ms on CI).

| Optimization | Impact | Where |
|---|---|---|
| **Array-based TT** — replaced `map[[16]byte]ttEntry` with `[1<<20]ttEntry` indexed by `hash & mask`. Stores `uint64` hash key for collision detection. | **~10x faster lookups** (100ns → 10ns per probe) | `search.go` |
| **int scores throughout search** — replaced all `float64` alpha/beta/bestScore with `int`. Sentinel values `maxScore=999999` / `minScore=-999999` replace `math.Inf`. | **~11x faster comparisons** (eliminates `math.Max`/`math.Min` float ops) | `search.go` |
| **Alpha-beta at root** — `rateAllMoves` now maintains alpha/beta bounds across root moves instead of using full `(-∞, +∞)` window for every move. | **~50% node reduction** (59k → 29k nodes) | `search.go` |
| **Direct square iteration** — `EvaluatePos` iterates 64 squares via `Board().Piece(sq)` instead of calling `Board().SquareMap()` which allocated a `map[Square]Piece` per call. | **Eliminated 43% of CPU time** (profile showed `SquareMap` dominant) | `eval.go` |
| **Array-based PST** — replaced `map[Color]map[PieceType][64]int` with `[3][3][7][64]int` (phase+color+piece+square). | **Eliminated nested map hashing** in eval hot path | `pst.go`, `eval.go` |
| **Array-based pieceValues** — replaced `map[PieceType]int` with `[7]int`. | **Eliminated map lookup** per piece per eval call | `eval.go` |
| **Array-based killer table** — replaced `map[uint8][]Move` with `[64]killerEntry`. | **Eliminated map hashing** in move ordering | `handler.go`, `eval.go` |
| **Integer endgame math** — replaced `math.Abs(4.5-float64(x))` with `absInt(9-x*2)`. | **Eliminated float64 ops** from evaluation | `eval.go` |
| **Hash caching** — compute `position.Hash()` once per node, reuse for TT lookup and store. | **Eliminated redundant hash computation** (~0.82µs per call) | `search.go` |
| **Aggressive LMR** — apply LMR after 4 moves (was half) with `moveScores[i] < 50` guard. | **~45% further node reduction** (29k → 16k nodes) | `search.go` |
| **Delta pruning in quiescence** — skip captures that can't raise score above alpha. | **Pruned futile captures** in tactical extensions | `quiescence_search.go` |
| **Quiescence int comparisons** — replaced `int(math.Max(float64(a), float64(b)))` with `if a > b`. | **Eliminated int↔float64 conversions** per quiescence node | `quiescence_search.go` |

**Benchmark: depth 5 from startpos (iterative deepening, depths 1–5)**

| State | Nodes | Time (CI) | Projected user time |
|-------|-------|-----------|-------------------|
| Before Round 2 | 59,044 | ~371ms | ~1.67s |
| After int scores + array TT + root α/β | 29,401 | ~217ms | ~0.98s |
| After direct iteration + array PST/killer | 29,401 | ~148ms | ~0.67s |
| After aggressive LMR + delta pruning | 16,001 | ~80ms | ~0.36s |

### Round 3 — Hashing & sort overhaul

| Optimization | Impact | Where |
|---|---|---|
| **Custom Zobrist hash (`fastPosHash`)** — replaced `position.Hash()` (md5-based, ~690 ns/call) with a purpose-built XOR hash using splitmix64 keys. | Eliminates md5 overhead in root + PV paths | `hashing.go` |
| **Incremental hash (`fastChildHash`)** — each minimax recursion node computes the child hash by XOR-ing only the move deltas (~5–10 ops) instead of rescanning all 64 squares. | **~3× faster than library hash** in the recursive hot path: 245 ms/1M vs 716 ms/1M | `hashing.go`, `search.go` |
| **Direct piece-by-ID lookup (`fastHashPieceByID`)** — keys indexed by raw `chess.Piece` int8, avoiding `Color()` + `Type()` method calls per square. | Eliminates 2 method calls per occupied square per hash | `hashing.go` |
| **Castle rights string scan** — one loop over the rights string (4 bytes max) instead of 4 `CanCastle()` calls. | Minor constant-factor improvement | `hashing.go` |
| **`sort.Slice` for move ordering** — replaced O(n²) selection sort with Go's built-in introsort via a `moveWithScore` struct. | O(n log n) move sorting, cleaner code | `search.go` |
| **TT bound flags** — `ttBoundExact/Lower/Upper` stored with every entry; `ttLookup` uses bounds to tighten the alpha-beta window even on non-exact hits. | More TT hits utilized for cutoffs | `transposition_table.go` |
| **Dedicated PV table** — `pvTable [ttSize]pvEntry` separate from TT; stores best-move UCI string per position+depth; `buildPVLine` reconstructs full predicted line. | Stable PV output; no eviction by interior nodes | `pv_store.go` |
| **Phase-weighted PST** — `EvaluatePos` computes opening/middle/end PST scores separately and blends them by `phaseWeights(totalMaterial)`. | Position evaluation tracks game phase | `eval.go` |

**Hash benchmark (1M calls, MODE=3):**

| Method | Time |
|--------|------|
| Library `position.Hash()` (md5) | ~716 ms |
| `fastPosHash` (full board scan) | ~1 222 ms |
| `fastChildHash` (incremental per node) | **~245 ms** |

**How to run the benchmark:**
```bash
go build -o barracuda .
MODE=4 ./barracuda
```

### Round 4 — Evaluation, Search Enhancements & Profiling

Identified via continued profiling and strategic feature implementation. Depth-7 from 
startpos improved significantly after integrating multiple synergistic optimizations.

| Optimization | Impact | Where |
|---|---|---|
| **Bitboard-based evaluator (fast EvaluatePos)** — replaces square-iteration path; extracts piece bitboards directly from position representation and iterates only occupied squares via low-bit loops. Retains LegacyEvaluatePos for comparative validation. | Faster material + PST accumulation; avoids per-square dispatch | `eval.go` |
| **Null-move pruning** — tries passing (null move) at reduced depth; if still outside window, prunes entire branch. Guards: checks disabled, non-pawn-material-only, minimum depth 3, `allowNull` recursion gate. | **~30–40% node reduction** in middlegame positions; skips zugzwang-prone endgames | `search.go` |
| **Aspiration windows** — root search at depths ≥ 3 uses narrow window `[prevScore − 30, prevScore + 30]` instead of `[-∞, +∞]`; re-searches full window if bound is exceeded. | **15–25% root search acceleration** on typical iterations | `search.go` |
| **Principal Variation Search (PVS)** — at root, first move uses full window; subsequent moves searched with null window `[alpha, alpha+1]` first. Re-search only if null window is exceeded. | **5–15% additional pruning** when combined with aspiration windows | `search.go` |
| **Incremental move selector (`pickNextBestMove`)** — one selection step per move loop instead of pre-sorting all moves with `sort.Slice`. Allows early cutoff and avoids O(n log n) cost when few moves are examined. | Asymptotically faster at nodes with expected cutoff; degrades on rare deep nodes | `search.go` |
| **PV prediction tracking (`predictedPVByHash`)** — after each iterative deepening depth, rebuild a map from position hash to next move in the completed PV. Apply `pvFollowBonus = +1000` to matches in move ordering. | Stable PV lines naturally prioritized; continues to top of move queue even if opponent deviates slightly | `pv_store.go`, `search.go` |
| **Quiescence stand-pat early-exit** — move generation deferred to after stand-pat checks; nodes pruned immediately don't pay move-generation cost. | Reduced allocation churn in qsearch | `quiescence_search.go` |
| **Profiling harness refinements** — migrated pawn-structure benchmark outside inner loop; split eval benchmarks to reflect fast vs. legacy path. | Cleaner benchmark labels and reduced measurement overhead | `profiling.go` |

**Observed performance:**
- Depth-7 node throughput improved from ~209k nodes/sec to ~350k nodes/sec (67% gain)
  on typical middle-game positions post-integration.
- Combined effect of null-move, aspiration, PVS: **25–35% node reduction** at depths 6–10.
- Bitboard evaluator parity verified via MODE=2 cross-validation during development.

### 14.1 Runtime Modes (`MODE=1..4`)

`main.go` currently multiplexes benchmark/debug entry paths behind the `MODE` env var:

- `MODE=1`: depth-7 benchmark search from start position; prints node/eval counters.
- `MODE=2`: debug path for `pawnStructure` evaluation testing on fixed FENs.
- `MODE=3`: lightweight repeated eval timing path.
- `MODE=4`: comprehensive hot-function profiling (`Benchmark()` in `profiling.go`).
- unset `MODE`: normal UCI loop.

PowerShell examples:

```powershell
$env:MODE = "1"; go run .
$env:MODE = "4"; go run .
```

### 14.2 Profiling Harness (`profiling.go`)

`Benchmark()` runs each hot function for 1,000,000 calls and prints per-call nanoseconds
plus an estimated CPU usage percentage.

Functions covered include:

- `ValidMoves`
- `Update`
- `EvaluatePos`
- `EvaluateMove`
- `fastPosHash`
- `fastChildHash`
- `ttLookup`
- `ttStore`
- `quiescence_search_depth1`

OS-specific CPU readers:

- Windows: `profiling_cpu_windows.go` uses `GetProcessTimes`.
- Non-Windows: `profiling_cpu_other.go` uses `runtime/metrics`.

---

## 15. Known Gaps & What to Implement Next

### High Priority

**Time management**
`SearchOptions` parses `wtime`, `btime`, `winc`, `binc` but they are never used in the
search loop. A proper time manager should allocate roughly `remaining_time / (moves_to_go + buffer)`
per move and interrupt iterative deepening via `stopSearch` when the budget expires. This is
essential for competitive play on fast time controls.

**History heuristic**
Track which quiet moves caused beta cutoffs across the whole search (not just at the same
depth like killers). Score moves by `history[from][to]` frequency and use it as a move-ordering
tiebreaker. Complements killers and iterative deepening history for even better node reduction.

### Moderate Priority

**Syzygy / EGTB endgame tables**
For positions with ≤6 pieces, tablebases provide exact results instantly. Even partial
tablebase support (probing at the root and in quiescence) would dramatically improve
endgame play and handling of rare fortresses or zugzwang positions where the engine
currently misjudges evaluation.

---

## 15. Configuration: Centralized Tuning Constants

All engine tuning parameters are defined in a single file: `config_variables.go`.

This ensures consistent configuration across the entire codebase and makes tuning clear and auditable.

### Configuration Sections

**Transposition Table:**
- `ttSize = 1 << 20` — 1,048,576 entry hash table
- Bound flags: `ttBoundExact`, `ttBoundLower`, `ttBoundUpper`

**PST Phases:**
- `pstStart`, `pstMiddle`, `pstEnd` — phase transition indices

**Search Bounds & Depth:**
- `maxScore / minScore = ±999999` — sentinel alpha/beta bounds
- `quiescenceDepth = 3` — max plies beyond main search depth

**Late Move Reduction (LMR):**
- `lmrMinDepth = 4` — minimum depth to apply LMR
- `lmrMoveIndex = 4` — first 4 moves at full depth, rest reduced

**Null-Move Pruning:**
- `nullMoveMinDepth = 3` — minimum depth for null-move try
- `nullMoveReduction = 2` — depth reduction when testing null move

**Aspiration Windows + PVS:**
- `aspiratingWindowMargin = 30` — centipawn band around previous score
- `aspirationMinDepth = 3` — apply aspiration window at depth ≥ 3
- `pvFollowBonus = 1000` — ordering bonus for predicted PV moves

**Quiescence & Move Ordering:**
- `deltaMargin = 200` — safety margin for delta pruning

**Profiling:**
- `benchmarkCalls = 1000000` — iterations per function in MODE=4

### Tuning Workflow

When experimenting with optimizations:
1. Adjust constants in `config_variables.go`
2. Run `MODE=1` benchmark to measure nodes/time impact
3. Log successful tuning results in `learn.md` and commit changes

## 16. Bitboard Library Architecture — `bitboardChess/`

### 16.1 Overview & Design Rationale

Barracuda uses a custom, high-performance bitboard library (`github.com/TheUtkarsh8939/bitboardChess`)
for position representation and move generation. This library was chosen to replace the external
chess library for the core engine due to superior performance and fine-grained control over
bitboard layout and hashing strategies.

**Key files in `bitboardChess/`:**

| File | Purpose |
|------|---------|
| `board.go` | `Board` struct (12 bitboards), `NewBoardFromFEN()`, `Board.Update()` |
| `move.go` | `Move` struct with boolean flags; move accessors S1(), S2(), Promo(), HasTag() |
| `movegen.go` | `GenerateValidMoves()` — the primary move generation function |
| `generated_magic_bitboards.go` | Pre-computed magic bitboard lookup tables (sliding pieces) |
| `generated_attack_tables.go` | Pre-computed attack masks for all piece types and squares |
| `compatibility_api.go` | Adapter layer exposing chess v2–style APIs for the engine |
| `board_test.go`, `move_test.go`, ... | Unit tests for all core functionality |

**Design philosophy:**

Rather than a heavyweight abstract `Position` type with methods, the bitboard library
exposes a minimal, performance-focused `Board` struct:

```go
type Board struct {
    WhitePawns, WhiteKnights, ..., BlackKing Bitboard    // 12 uint64 fields
    castleRights, enPassant, halfmoveCount, fullmoveNumber uint8
    turn bool  // true=White, false=Black
}
```

The engine code doesn't directly manipulate bitboards; instead, Barracuda uses a **compatibility layer**
(`compatibility_api.go`) that wraps the Board and exposes the familiar chess v2 API:

```go
// Compatibility types (all defined in compatibility_api.go)
type Position struct { board Board }
type Game struct { position *Position; ... }
type Move struct { ... }  // Full move metadata
type Piece, Square, Color, PieceType, ...  // All familiar enums
```

This decoupling allows the core engine files (`search.go`, `eval.go`, `hashing.go`) to remain
largely unchanged after the library swap — they compile against the same API signatures,
but now backed by the fast bitboard implementation.

### 16.2 Core Data Structures

#### Bitboard (`uint64`)

A **bitboard** is a 64-bit unsigned integer where each bit represents one square on the chess board.
Bit `i` (0–63) corresponds to square `i`. The layout follows the LSB=H1 convention in the library's
internal representation, but the compatibility layer re-maps this as needed.

```
Bit 0  → H1 (LSB position)
Bit 7  → A1
Bit 8  → H2
...
Bit 63 → A8 (MSB position)
```

Bitboard operations are extremely fast:

```go
// Set bit at square i
bb |= (1 << uint(i))

// Check if square i is occupied
if bb&(1 << uint(i)) != 0 { ... }

// Iterate all set bits (using low-bit isolation)
for bb != 0 {
    idx := bits.TrailingZeros64(bb)
    // Process square idx
    bb &= bb - 1  // Clear lowest set bit
}
```

#### Board (`bitboardChess.Board`)

```go
type Board struct {
    // Piece bitboards (12 total: 6 White piece types + 6 Black piece types)
    WhitePawns, WhiteKnights, WhiteBishops, WhiteRooks, WhiteQueens, WhiteKing Bitboard
    BlackPawns, BlackKnights, BlackBishops, BlackRooks, BlackQueens, BlackKing Bitboard

    // Game state
    castleRights   uint8              // 4-bit field encoding K/Q/k/q rights
    enPassant      uint8              // Square index (0–63) or 255 (none)
    halfmoveCount  uint8              // Halfmove clock (reset on pawn move / capture)
    fullmoveNumber uint8              // Full move counter (increments after Black moves)
    turn           bool               // true=White, false=Black
}
```

Each piece type occupies its own bitboard. To find all White's pieces, OR the White bitboards together:

```go
whiteAll := board.WhitePawns | board.WhiteKnights | ... | board.WhiteKing
```

The `castleRights` field encodes rights as a 4-bit mask:
- Bit 0: White King-side (K)
- Bit 1: White Queen-side (Q)
- Bit 2: Black King-side (k)
- Bit 3: Black Queen-side (q)

#### Move (`bitboardChess.Move`)

```go
type Move struct {
    From       uint8  // Source square (0–63)
    To         uint8  // Destination square (0–63)
    PieceType  uint8  // Moving piece using 1-based indexing: Pawn=1, Knight=2, Bishop=3, Rook=4, Queen=5, King=6
    promoteTo  uint8  // Promotion piece type (0 if no promotion)
    isCapture  bool
    isCheck    bool
    isEnPassant bool
    isCastle   bool
}
```

Access methods (defined in compatibility layer):
- `Move.S1()` → source square as `chess.Square` enum
- `Move.S2()` → destination square as `chess.Square` enum
- `Move.Promo()` → promotion piece type (or `NoPieceType`)
- `Move.HasTag(tag)` → true if move has flag (Capture, Check, EnPassant, etc.)

### 16.3 Compatibility API Layer

The `bitboardChess/compatibility_api.go` file (576 lines) provides a complete adapter layer
that translates the engine's chess v2 API calls onto the bitboard implementation. Key types:

```go
// Type aliases for familiar enumerations
type Color uint8
type Piece uint8
type PieceType uint8
type Square uint8
type MoveTag uint8

// Wrapper struct for position
type Position struct {
    board Board
}

// Game struct (similar to chess.Game)
type Game struct {
    position *Position
    moves    []Move
}

// Move metadata provider
type Move struct { From, To uint8; ... }

// Full chess v2-compatible API
func (p *Position) Turn() Color
func (p *Position) ValidMoves() []Move
func (p *Position) Update(move *Move) *Position
func (p *Position) Board() *Board
func (p *Position) Status() (GameStatus, bool)  // Terminal position detection
func (p *Position) MarshalBinary() ([]byte, error)
func (p *Position) CastleRights() string  // "KQkq" format
func (p *Position) EnPassantSquare() Square
```

**Key implementation detail: `MarshalBinary()` Index Mapping**

The engine's PST evaluation and bitboard utilities expect a specific bitboard layout.
The `MarshalBinary()` function must account for the library's internal square indexing
vs. the engine's expected layout. This is handled by the `mirrorFilesInRanks()` helper,
which flips bitboard bit patterns to realign file ordering for PST lookups.

```go
func (p *Position) MarshalBinary() ([]byte, error) {
    bbs := p.board.GetBitboards()  // 12 bitboards
    for i := range &bbs {
        bbs[i] = mirrorFilesInRanks(bbs[i])  // Adjust for PST indexing
    }
    // ... encode bbs into [96]byte array ...
    return result, nil
}
```

### 16.4 Move Generation Algorithm

`GenerateValidMoves(board Board) []Move` and `GenerateValidMovesInto(board Board, moveBuffer []Move)` are the primary move generation entry points. `GenerateValidMovesInto` prevents excessive slice allocations during deep recursive searches by efficiently reusing a pre-allocated capacity-64 buffer.

**High-level Direct Legal Move Generation strategy:**

The engine aggressively pre-calculates legal bounds (check masks, pins) to directly generate legal moves:
1. **Pre-compute Constraints:** Computes `checkers` and `pinRays` before attempting any generation.
2. **Double Check Evasion:** If there's a double check, skip all piece generation except the King.
3. **Iterate piece types** (Pawns, Knights, Bishops, Rooks, Queens, Kings)
4. **For each piece of the moving side:**
   - Constrain moves onto the `checkMask` (if in single check) or respective pin rays (if pinned) to avoid pseudo-legal generation that leaves the king in check.
   - Use pre-computed magic bitboards or attack tables to find all legal destination squares.
5. **Escape moves from check** (if the side is in check):
   - Generate all moves that block or capture the attacking piece
6. **Encode each move** as a `Move` struct with flags (Capture, Check, EnPassant, Castle). Note that check flags can be toggled to save computation time via `annotateChecksInMoveGen`.

**Attack table usage:**

Pre-computed lookup tables (`generated_attack_tables.go`, `generated_magic_bitboards.go`)
provide instant attack masks:

```go
// Pawn attacks from square sq (for side color) — simple lookup
pawnAttacks := pawnAttackTable[color][sq]

// Knight attacks from square sq
knightAttacks := knightAttackTable[sq]

// Sliding piece attacks (bishops/rooks/queens) — magic bitboard lookup
// Accounts for occupied squares (blockers) to compute legal rays
rook Attacks := rookAttackTable(sq, occupancy)
```

The magic bitboard technique uses pre-computed hash functions to instantly compute
sliding-piece attacks even with obstacles. This is significantly faster than scanning
rays square-by-square.

### 16.5 Board State Representation

**FEN Parsing (`NewBoardFromFEN`):**

```go
func NewBoardFromFEN(fen string) *Board
```

Parses a standard FEN string and initializes all 12 bitboards directly, perfectly populating the core `Board` structure. It parses algebraic notation to the `enPassant` integer, extracts boolean side-to-move turn parameters, maps string castling text to the internal 4-bit `castleRights` mask, and extracts halfmove/fullmove clocks in one pass. It provides proper error handling and returns `nil` for malformed FEN strings.

**Move Application (`Board.Update`):**

```go
func (b *Board) Update(m Move) (Board, error)
```

Applies a move to the board, handling:
- Piece placement (source → destination)
- Capture removal (clearing opponent's piece)
- En passant capture (removing pawn one rank behind the destination)
- Castling (moving both king and rook)
- Promotion (replacing pawn with promoted piece)
- Castle rights updates (lost when king/rook moves)
- Halfmove clock reset (on pawn move or capture)
- Fullmove counter increment (after Black's move)

Returns the new `Board` state or an error if the move is invalid.

### 16.6 Integration with Engine

The engine interacts with the bitboard library exclusively through the compatibility layer:

```go
import chess "github.com/TheUtkarsh8939/bitboardChess"

// In search.go
func rateAllMoves(position *chess.Position, depth uint8, ...) (*chess.Move, int) {
    moves := position.ValidMoves()      // Returns []chess.Move
    for _, move := range moves {
        child := position.Update(&move)  // Returns new *chess.Position
        score := minimax(child, depth-1, ...)
    }
}
```

The engine never manipulates bitboards directly. All bitboard logic is encapsulated within
the library — the engine's code remains clean and portable.

---

## 17. Opening Book Integration — `opening_handler.go`

The engine integrates ECO opening book support via the external `github.com/corentings/chess/v2/opening` library. 
Rather than rewriting or replacing this dependency, a **legacy bridge pattern** is used to maintain compatibility 
with the old chess library's `opening.BookECO.Possible(moves []*chess.Move)` API while the rest of the engine 
uses the new bitboard library.

### 17.1 ECO Opening Book System

The `opening.BookECO` database contains hundreds of thousands of known opening lines organized by ECO code 
(Encyclopaedia of Chess Openings). Each opening is a sequence of moves drawn from master games.

**How the book integration works:**

1. **Initialize the book once at startup**:
   ```go
   book := opening.NewBookECO()  // Loads ECO database from embedded resource
   ```

2. **At each turn, query the book for the next move**:
   ```go
   nextMove := findNextMove(uciMoveHistory, book)
   ```
   - Input: `uciMoveHistory` is a `[]string` of UCI move tokens (e.g., `["e2e4", "c7c5", "g1f3"]`)
   - Output: A single UCI move token (e.g., `"d2d4"`) or empty string if no book move exists

3. **In the UCI command loop** (`main.go`):
   ```go
   if len(moveArray) < 8 {  // Consult book early game
       bookMove := findNextMove(moveArray, book)
       if bookMove != "" {
           fmt.Printf("bestmove %s\n", bookMove)
           return  // Skip expensive search
       }
   }
   iterativeDeepening(pos, depth, pst, isWhite)  // Fall back to search
   ```

### 17.2 Legacy Bridge Pattern

The core challenge: the bitboard library's engine now uses UCI move strings (`[]string`), 
but `opening.BookECO.Possible()` expects the old chess library's `[]*chess.Move` objects. 
Rather than modifying the opening package, a **bridge function** converts between the two:

**`buildLegacyMovesFromUCI(uciMoves []string) ([]*chess.Move, error)`:**

```go
func buildLegacyMovesFromUCI(uciMoves []string) ([]*chess.Move, error) {
    game := chess.NewGame()  // Create temporary old-library game
    legacyMoves := make([]*chess.Move, 0, len(uciMoves))
    
    for _, token := range uciMoves {
        // Decode UCI token into old chess.Move using old library's API
        mv, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), token)
        if err != nil {
            return nil, err
        }
        legacyMoves = append(legacyMoves, mv)
        
        // Apply move to keep the game state synchronized
        if err := game.Move(mv, &chess.PushMoveOptions{}); err != nil {
            return nil, err
        }
    }
    return legacyMoves, nil
}
```

This function:
1. Creates a temporary `chess.Game` to track position state
2. For each UCI move token (e.g., "e2e4"), decodes it into an old `chess.Move`
3. Applies each move to the temporary game
4. Returns the list of old-format moves

The decoder uses the current position state to disambiguate moves (e.g., multiple knights can 
move to the same square in rare positions, though UCI should already be unambiguous).

**`findNextMove(uciMoves []string, book *opening.BookECO) string`:**

```go
func findNextMove(uciMoves []string, book *opening.BookECO) string {
    // Convert UCI move history to old library format
    moves, err := buildLegacyMovesFromUCI(uciMoves)
    if err != nil {
        return ""
    }

    // Query opening book
    openings := book.Possible(moves)
    if len(openings) == 0 {
        return ""  // No opening line matches
    }

    // Extract moves from the first matching opening
    pgn := openings[0].PGN()
    pgnMoves := parsePGNMoves(pgn)
    if len(pgnMoves) <= len(uciMoves) {
        return ""  // Book line has ended
    }

    // Return the next move in UCI format (PGN is parsed as UCI)
    return pgnMoves[len(uciMoves)]
}
```

This function:
1. Converts UCI move history to old chess.Move format via `buildLegacyMovesFromUCI()`
2. Queries the opening book with the converted moves
3. If matches exist, parses the opening's PGN and returns the next move in the sequence
4. Returns empty string if the book line has ended or no matches found

**Why this pattern is necessary:**

The opening book API is locked to `Book.Possible(moves []*chess.Move)` — we cannot change external signatures. 
By keeping a minimal legacy import (`github.com/corentings/chess/v2` + `github.com/corentings/chess/v2/opening` 
only for the opening package) and converting moves on-demand via the bridge, we avoid:
- Rewriting the opening book library (impossible — it's external)
- Forcing the entire engine to import the old library (performance regression)
- Duplicating opening book data (massive binary burden)

The cost is minimal: the bridge is only called 4–8 times per game (opening book rarely applies beyond move 8), 
so the temporary `chess.Game` allocation and move decoding happen infrequently.

### 17.3 UCI Move History Tracking

The engine's main loop (`main.go`) now tracks move history as **UCI strings** instead of Move objects:

```go
var moveArray []string  // e.g., ["e2e4", "c7c5", "g1f3", ...]
```

**`applyMovesUCI(moveList []string, position *chess.Position)`:**

For each UCI move token in the input (from the UCI `position` command), decode it and apply it.
The decoded move is converted back to UCI format and appended to `moveArray`:

```go
for _, token := range moveList {
    mv, _ := chess.Notation.Decode(chess.UCINotation{}, position, token)
    position = position.Update(mv)
    moveArray = append(moveArray, token)
}
```

This approach keeps `moveArray` as pure UCI strings, avoiding any dependency on the chess Move type 
in the core loop.

**Benefits of this approach:**

1. **Cleaner data flow:** The engine logic doesn't care about Move struct details; it works with move tokens
2. **Reduced coupling:** Main loop doesn't need to know chess.Move layout or memory semantics
3. **Efficient bridging:** To query the opening book, `findNextMove()` converts the string batch once, 
   rather than converting individual Move objects throughout the loop
4. **Stateless opening queries:** The opening book function is a pure function mapping `[]string → string`, 
   with no side effects or parameter-passing complications

**`parsePGNMoves(pgn string) []string`:**

Extracts individual moves from a PGN (Portable Game Notation) string:

```go
func parsePGNMoves(pgn string) []string {
    tokens := strings.Fields(pgn)
    var moves []string

    for _, token := range tokens {
        // Skip move numbers (e.g., "1.", "2.", "...")
        if strings.HasSuffix(token, ".") {
            continue
        }
        // Skip any other non-move tokens
        if token == "" || token == "*" {
            continue
        }
        moves = append(moves, token)
    }
    return moves
}
```

The parser:
- Splits the PGN by whitespace
- Skips move numbers (tokens ending with `.`, e.g., `1.`, `2.`)
- Skips metadata and annotations
- Returns a slice of move strings in algebraic notation (e.g., `["e2e4", "c7c5", "Nf3", ...]`)

The parser handles both UCI-style moves (`e2e4`) and standard algebraic notation (`Nf3`, `O-O`).

### 17.4 Integration Checklist

- ✅ `opening_handler.go` imports both old (`github.com/corentings/chess/v2`) and new (`github.com/TheUtkarsh8939/bitboardChess`) libraries
- ✅ Engine core files (`search.go`, `eval.go`, etc.) import only the new bitboard library (via compatibility layer)
- ✅ Move history in `main.go` is `[]string` (UCI moves)
- ✅ `findNextMove()` signature is `(uciMoves []string, book) → string`
- ✅ Bridge function `buildLegacyMovesFromUCI()` handles conversion on-demand
- ✅ No opening book functionality is modified; only the call site is adapted

---

## 18. Build Instructions

### Standard build

```powershell
go build -o Barracuda.exe .
```
### Profile-Guided Optimization (PGO) build

PGO lets the Go compiler inline and optimize hot paths based on a real CPU profile.

**Step 1 — Generate a profile.** Create a temporary `bench_test.go`:
```go
package main
import (
    "testing"
    "github.com/corentings/chess/v2"
)
func BenchmarkSearch(b *testing.B) {
    fen, _ := chess.FEN("r1b2rk1/pp1pqppp/2p5/3nP3/1b1Q1P2/2N5/PPPBB1PP/R3K2R b KQ - 2 12")
    game  := chess.NewGame(fen)
    pst   := initPST()
    for i := 0; i < b.N; i++ {
        rateAllMoves(game.Position(), 5, pst, false)
    }
}
```

> Note: `go test` cannot import `package main` directly. To work around this, temporarily
> change the package declaration in all `.go` files to `package barracuda`, run the bench,
> then change back — or use a separate sub-package.

**Step 2 — Build with profile:**
```powershell
go build -pgo=default.pgo -o Barracude-V4.exe .
```

### Run the benchmark test harness

In `main.go`, swap the commented benchmark `main` back in:
```powershell
go run .
# Outputs: "info depth N score cp X" lines + total nodes + elapsed time
```

### Connect to a GUI (UCI mode)

Build the binary, then point any UCI-compatible GUI (Arena, Cutechess, BankSiaBot) at the
executable. The engine will print `uciok` on startup and handle `position`/`go`/`stop` commands.
