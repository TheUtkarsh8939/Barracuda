# Barracuda — Implementation Deep Dive

> A Go chess engine written from scratch. Uses `github.com/corentings/chess/v2` for board
> representation and legal move generation. Implements a full adversarial search stack
> on top of that library.

---

## Table of Contents

1. [Project Structure](#1-project-structure)
2. [How a Chess Engine Works — The Mental Model](#2-how-a-chess-engine-works--the-mental-model)
3. [Search — `search.go`](#3-search--searchgo)
   - 3.1 Minimax & Alpha-Beta
   - 3.2 Late Move Reduction (LMR)
   - 3.3 Transposition Table
   - 3.4 Iterative Deepening
   - 3.5 Root Search (`rateAllMoves`)
4. [Quiescence Search — `quiescence_search.go`](#4-quiescence-search--quiescence_searchgo)
5. [Evaluation — `eval.go`](#5-evaluation--evalgo)
   - 5.1 Material
   - 5.2 Piece-Square Tables
   - 5.3 Castling Rights
   - 5.4 Endgame King Centralization
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
15. [Known Gaps & What to Implement Next](#15-known-gaps--what-to-implement-next)
16. [Build Instructions](#16-build-instructions)

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
go.mod / go.sum       — module file (single dependency: corentings/chess)
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

1. Calls `rateAllMoves` at that depth.
2. Records the best move in `lastBestMoves` (a `map[Move]bool`).
3. Emits a UCI `info depth X score cp Y` line.
4. Checks `stopSearch` channel — if the GUI sends "stop", the last complete depth's
   best move is returned immediately.

The earlier iterations are not wasted: best moves from depth N inform move ordering at
depth N+1, dramatically increasing alpha-beta cutoffs.

### 3.5 Root Search (`rateAllMoves`)

```go
func rateAllMoves(position, depth, pst, isWhite) (*chess.Move, int)
```

Loops over every legal move at the root, calls `minimax` for each, and tracks the best.
At the root level, castling moves get a **+200 bonus** applied to their returned scores
to encourage castling when positions are otherwise roughly equal.

The root search now maintains its own **alpha-beta window** across root moves. As each
move is evaluated, the alpha (for White) or beta (for Black) bound tightens, allowing
`minimax` to prune more branches for later root moves:

```go
alpha := minScore
beta := maxScore
for _, move := range moves {
    score := minimax(pos.Update(move), depth-1, !isWhite, alpha, beta, pst)
    // ... update bestScore ...
    if isWhite { alpha = max(alpha, score) }
    else       { beta  = min(beta,  score) }
}
```

**Previous version:** Every root move was searched with a full `(-∞, +∞)` window,
meaning no pruning could occur for later root moves even when an excellent move was
already found. Adding the alpha-beta window at root reduced total nodes by ~50%.

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

The `depth` parameter limits quiescence to 1 extra ply currently to prevent explosion
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

**Previous version:** No delta pruning; every capture/check was searched unconditionally.
Also used `math.Max(float64(alpha), float64(eval))` for integer comparisons, which
incurred expensive int→float64 conversions at every quiescence node. These have been
replaced with simple `if` comparisons.

---

## 5. Evaluation — `eval.go`

```go
func EvaluatePos(position, pst) int
```

Returns a score in **centipawns** from White's perspective (positive = good for White,
negative = good for Black). Four components:

### 5.1 Material

```go
var pieceValues = [7]int{
    0,      // NoPieceType (index 0)
    100000, // King
    900,    // Queen
    500,    // Rook
    300,    // Bishop
    300,    // Knight
    100,    // Pawn
}
```

Every piece on the board contributes its value. Black pieces subtract, White add.

**Previous version:** `pieceValues` was a `map[chess.PieceType]int`. Each lookup required
Go map hashing (~10–15ns). Switching to a flat `[7]int` array indexed by `PieceType`
(which is an `int8` enum: 0=NoPieceType, 1=King, ..., 6=Pawn) eliminates this overhead.

### 5.2 Piece-Square Tables

Each piece gets a **positional bonus** based on which square it occupies:

```go
positionalAdv := pst[pieceColor][pieceType][sq]
```

The PSTs encode human chess knowledge: knights are better in the center, rooks want
open files and the 7th rank, etc. See `pst.go` for the full tables.

**Board iteration:** `EvaluatePos` iterates all 64 squares using `Board().Piece(sq)`
directly, skipping empty squares immediately:

```go
for sq := 0; sq < 64; sq++ {
    v := board.Piece(chess.Square(sq))
    if v == chess.NoPiece { continue }
    // ... score the piece
}
```

**Previous version:** Used `Board().SquareMap()` which allocates a `map[Square]Piece`
on every call. CPU profiling showed `SquareMap()` consuming ~43% of total search time.
Direct iteration avoids the map allocation entirely. The chess library's `Piece(sq)`
checks 12 bitboards internally, so 64 × 12 = 768 bitboard checks — but this is cheaper
than the map allocation and hashing overhead that `SquareMap` required.

### 5.3 Castling Rights

```
Still has kingside castle right  → +50
Still has queenside castle right → +40
(symmetric penalty for Black)
```

Losing the right to castle permanently is a king safety risk. These bonuses degrade
naturally as the engine trades away castling rights.

### 5.4 Endgame King Centralization

In the endgame, active kings are critical for escorting pawns and delivering checkmate.
The eval rewards the side ahead for having a more central king:

```go
const maxMaterial = 7800  // all non-king material at game start
endGameIndex := maxMaterial - totalMaterial
if endGameIndex > 4900 {
    blackDist := absInt(9-blackKingFile*2) + absInt(9-blackKingRank*2)
    whiteDist := absInt(9-whiteKingFile*2) + absInt(9-whiteKingRank*2)
    smartEndgameFactor := endGameIndex - 4900
    score += (-whiteDist + blackDist) * smartEndgameFactor / 4
}
```

- `maxMaterial = 7800` (2×900 + 4×500 + 4×300 + 4×300 + 16×100)
- Factor is **0** until ~4900 centipawns of material have been traded (halfway point)
- After that it scales linearly, reaching ~29 at full endgame
- Kings start at -200000 to cancel out of `totalMaterial` so king captures don't trigger
  the endgame factor prematurely

**Previous version:** Used `math.Abs(4.5-float64(king.x))` with float64 arithmetic
throughout. Now uses a pure integer approach: coordinates are doubled (center at 9,9
instead of 4.5,4.5) and `absInt()` replaces `math.Abs()`, eliminating all float64
operations from the evaluation hot path.

---

## 6. Move Ordering — `eval.go` (`EvaluateMove`)

```go
func EvaluateMove(move *chess.Move, position *chess.Position, depth uint8) int
```

Move ordering is critical for alpha-beta performance. Better-ordered moves cause more
cutoffs. `EvaluateMove` assigns a priority score to each move before sorting. It is
called **once per move** before the sort (scores are cached in `moveScores[]`), not
inside the comparator. Moves are then sorted descending by score using `sort.Slice`.

| Heuristic | Bonus | Notes |
|-----------|-------|-------|
| Iterative deepening history | +700 | Best at any shallower depth — almost always good here too |
| Queen promotion | +900 | Scored by promoted piece value |
| MVV-LVA captures | variable | `pieceValues[victim] − pieceValues[attacker]`, floor 30 |
| Rook promotion | +500 | |
| Bishop / Knight promotion | +300 | |
| Killer moves | +200 | Caused beta cutoff in sibling node at same depth |
| Castling | +150 | King safety; also gets +200 root bonus in `rateAllMoves` |
| Moves that give check | +100 | Forcing; usually narrows opponent options |

*Sorting:* `sort.Slice` (Go's built-in introsort, O(n log n)) replaced a previous O(n²)
selection sort. A `moveWithScore` struct carries both move and score through the sort,
then the two parallel slices are reconstructed in sorted order afterward.

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

`initPST()` returns a `[3][7][64]int` array indexed by `[Color][PieceType][Square]`.
Color 1 = White, Color 2 = Black; PieceType 1 = King, ..., 6 = Pawn.

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
| King | All zeros (safety handled by castling rights bonus + endgame logic) |

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
Note: time fields are parsed but **not yet used** for time management.

**`isCastlingMove`:** Detects castling by checking king origin+destination squares
(E1→G1/C1 for White, E8→G8/C8 for Black). Used both in move ordering (+40) and in the
root search (+200 bonus).

**`InsertionSort`:** An O(n²) alternative to `sort.Slice` written for benchmarking.
Currently unused in production (selection sort is used in `search.go` instead because
it keeps `moves[]` and `moveScores[]` in sync without extra data structures).

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
| Pre-compute `moveScores[]` once before sort (was calling `EvaluateMove` O(n log n) times) | **~2x speedup** | `search.go` |
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
| **Array-based PST** — replaced `map[Color]map[PieceType][64]int` with `[3][7][64]int`. | **Eliminated nested map hashing** in eval hot path | `pst.go`, `eval.go` |
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
BENCH=1 ./barracuda
```

---

## 15. Known Gaps & What to Implement Next

### High Priority

**Principal Variation Search (PVS)**
After searching the first move at full window `(alpha, beta)`, search remaining moves
with a zero-width window `(alpha, alpha+1)`. If the result falls outside the window
(rare with good ordering), re-search with the full window. This is extremely effective
combined with good move ordering and can further halve the number of nodes searched.

**Null Move Pruning**
Before searching all children, try passing (making no move) and searching at `depth-3`.
If the null-move result still exceeds beta, the current position is so overwhelming that
a real move also will — prune immediately. Should be disabled in zugzwang-prone endgames.

**Time management**
`SearchOptions` already parses `wtime`, `btime`, `winc`, `binc` but they are never used.
A proper time manager should allocate roughly `remaining_time / (moves_to_go + buffer)`
per move and interrupt the search via `stopSearch` when the budget expires.

### Moderate Priority

**Aspiration windows**
Run `rateAllMoves` with a narrow alpha-beta window around the previous iteration's score
(e.g. ±50 centipawns) instead of `(−∞, +∞)`. If the search fails high or low, re-search
with a wider window. This dramatically reduces the root search cost on most iterations.

**History heuristic**
Track which quiet moves caused beta cutoffs across the whole search (not just at the same
depth like killers). Score moves by `history[from][to]` and use it to bias move ordering.
Complements killers and iterative deepening history.

**Syzygy / EGTB endgame tables**
For positions with ≤6 pieces, tablebases give exact results instantly. Even partial
tablebase support (probing at the root) dramatically improves endgame play.

---

## 16. Build Instructions

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
