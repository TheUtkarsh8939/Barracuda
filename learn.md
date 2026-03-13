# Barracuda — Implementation Deep Dive

> A Go chess engine written from scratch. Uses `github.com/corentings/chess` for board
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
9. [Utilities — `misc.go`](#9-utilities--miscgo)
10. [UCI Protocol — `main.go` & `ucihelper.go`](#10-uci-protocol--maingo--ucihelpergo)
11. [Data Flow: One Full Search Cycle](#11-data-flow-one-full-search-cycle)
12. [Performance Notes & Optimization History](#12-performance-notes--optimization-history)
13. [Known Gaps & What to Implement Next](#13-known-gaps--what-to-implement-next)
14. [Build Instructions](#14-build-instructions)

---

## 1. Project Structure

```
main.go               — UCI command loop + benchmark test harness
search.go             — minimax, alpha-beta, iterative deepening, TT, LMR
eval.go               — static position evaluation + move ordering scorer
quiescence_search.go  — quiescence search (capture/check extension)
pst.go                — piece-square tables (PSTs) + mirror helper
handler.go            — killer move table management
misc.go               — Move struct, SearchOptions, InsertionSort, isCastlingMove
ucihelper.go          — parseGoCmd (UCI "go" command tokenizer)
go.mod / go.sum       — module file (single dependency: corentings/chess)
```

**Dependency:** `github.com/corentings/chess` handles the board, legal move generation,
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
if i > 3 && depth >= 3 && moveScores[i] < 50 {
    score = minimax(..., depth-2, ...)      // Reduced search
    if score > alpha {                      // (or score < beta for minimizer)
        score = minimax(..., depth-1, ...)  // Full re-search if promising
    }
}
```

If a reduced-depth search looks promising (beats alpha for maximizer, or beta
for minimizer), a full-depth re-search confirms it. This saves significant time
since well-ordered late moves almost never turn out to be best.

LMR is only applied at `depth >= 3` to avoid reducing searches that are already shallow.
The `moveScores[i] < 50` guard ensures captures, checks, promotions, and killer moves
(which all score ≥ 50) are never reduced — only quiet moves with low move-ordering scores.

**Previous version:** LMR was applied after `len(moves)/2` and used `bestScore` instead
of `alpha`/`beta` for the re-search trigger. The more aggressive threshold (after 4 moves)
combined with the score guard produces ~45% fewer nodes with equivalent or better play.

### 3.3 Transposition Table

```go
const ttSize = 1 << 20  // ~1M entries
const ttMask = ttSize - 1

type ttEntry struct {
    hashKey uint64   // upper bits for collision detection
    score   int
    depth   uint8
}
var transpositionTable [ttSize]ttEntry
```

The transposition table uses an **array-based** design indexed by `hash & ttMask`
(equivalent to `hash % ttSize` but faster since `ttSize` is a power of 2). This replaces
the previous `map[[16]byte]ttEntry` which had ~100ns per lookup due to Go map hashing
overhead. Array lookups are ~10ns — roughly a 10× improvement on this critical hot path.

The 16-byte **Zobrist hash** from the chess library is converted to `uint64` using the
first 8 bytes via `encoding/binary.LittleEndian.Uint64`. The full `uint64` is stored in
`hashKey` for collision detection — two positions can map to the same array index but
will have different hash keys.

```go
func ttLookup(h uint64, depth uint8) (int, bool) {
    idx := h & ttMask
    entry := &transpositionTable[idx]
    if entry.hashKey == h && entry.depth >= depth {
        return entry.score, true
    }
    return 0, false
}
```

The critical guard is `entry.depth >= depth`. Without it, a result computed at depth 1
can be returned when depth 6 is needed — producing completely wrong moves. This was a
real bug that was fixed. Entries are stored with depth:

- Terminal positions (checkmate/stalemate): depth `255` (exact forever)
- Leaf nodes (quiescence result): depth `0`
- Interior nodes: the actual depth they were searched to

The hash is computed **once** per node and cached in a local variable, avoiding the
previous pattern of calling `position.Hash()` twice (once for TT lookup, once for store).

The TT **persists across iterative deepening iterations**. Entries are not cleared
between depths because the depth guard makes them safe to reuse.

**Previous version:** Used `map[[16]byte]ttEntry` with Go map hashing. Each TT lookup
required map key hashing (~100ns) and the map created GC pressure from dynamic allocation.

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
func EvaluateMove(move, position, depth) int
```

Move ordering is critical for alpha-beta performance. Better-ordered moves cause more
cutoffs. `EvaluateMove` assigns a priority score to each move before sorting. It is
called **once per move** before the sort (scores are cached in `moveScores[]`), not
inside the comparator.

| Priority | Heuristic | Points |
|----------|-----------|--------|
| 1st | Iterative deepening history (was best at a shallower depth) | +700 |
| 2nd | Queen promotion | +900 |
| 3rd | MVV-LVA captures (victim value − attacker value, floor 30) | variable |
| 4th | Rook promotion | +500 |
| 5th | Killer moves (caused cutoffs at same depth in sibling nodes) | +70 |
| 6th | Bishop/Knight promotion | +300 |
| 7th | Moves that give check | +50 |
| 8th | Castling | +40 |

**MVV-LVA** (Most Valuable Victim – Least Valuable Attacker): prefers captures where the
captured piece is worth more than the capturing piece. Example: pawn takes queen = 900−100 = 800.
If the net is negative (losing trade), the score floors at 30 so captures still come
before quiet moves.

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

## 9. Utilities — `misc.go`

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

## 10. UCI Protocol — `main.go` & `ucihelper.go`

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

## 11. Data Flow: One Full Search Cycle

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

## 12. Performance Notes & Optimization History

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

**How to run the benchmark:**
```bash
go build -o barracuda .
BENCH=1 ./barracuda
```

---

## 13. Known Gaps & What to Implement Next

### High Priority

**TT entry flags (Exact / Lowerbound / Upperbound)**
Currently every TT entry is stored as an exact score. In reality, alpha-beta may not
have explored all branches:
- A node that caused a beta cutoff only provides a lower bound on the true score.
- A node where no move beat alpha only provides an upper bound.
Storing and correctly using these flags (standard `EXACT / LOWERBOUND / UPPERBOUND` enum)
will allow the TT to be used for window narrowing, not just exact lookup.

**Principal Variation Search (PVS)**
After searching the first move at full window `(alpha, beta)`, search remaining moves
with a zero-width window `(alpha, alpha+1)`. If the result falls outside the window
(rare with good ordering), re-search with full window. This is extremely effective
combined with good move ordering and can further halve the number of nodes searched.

**Null Move Pruning**
Before searching all children, try passing (making no move) and searching at `depth-3`.
If the null-move result still exceeds beta, the position is so overwhelming that a real
move will too — prune immediately. Effective in most non-endgame positions; should be
disabled in zugzwang-prone endgames.

**Time management**
`SearchOptions` already parses `wtime`, `btime`, `winc`, `binc` but they are never used.
A proper time manager should allocate roughly `remaining_time / (moves_to_go + buffer)`
per move and interrupt the search via `stopSearch` when the budget expires.

### Moderate Priority

**Array-based transposition table**
The current `map[[16]byte]ttEntry` has ~100ns per lookup due to Go map hashing overhead.
A fixed-size `[tableSize]ttEntry` array indexed by `hash % tableSize` (common in chess engines)
brings this down to ~10ns. The smaller size means collisions (handled by age replacement).

**int throughout the search**
`minimax` uses `float64` for alpha/beta/scores. All real evaluation values are integers.
Switching to `int` eliminates `math.Max`/`math.Min` float ops and makes comparison faster.

**Quiescence search depth**
Currently capped at 1 ply. For tactical positions this can miss important captures.
A depth of 3–5 is more typical; add delta-pruning to keep it fast.

---

## 14. Build Instructions

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
    "github.com/corentings/chess"
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
