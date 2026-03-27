# Barracuda

Barracuda is a UCI chess engine written in Go on top of `github.com/corentings/chess/v2`.

It currently uses:

- Iterative deepening alpha-beta search
- Quiescence search with stand-pat and delta pruning
- Late Move Reduction (LMR)
- Null-move pruning
- Aspiration windows + root PVS
- Killer moves + iterative-depth move history + PV-follow ordering
- Array-based transposition table and dedicated PV table

## Build

```powershell
go build -o Barracuda.exe .
```

## Run Modes

`main.go` supports multiple startup modes via `MODE`:

```powershell
# UCI mode (default)
go run .

# Fixed depth-7 benchmark from start position
$ENV:MODE=1; go run .

# Eval parity/debug path on a fixed FEN
$ENV:MODE=2; go run .

# Evaluation micro-benchmark
$ENV:MODE=3; go run .

# Hot-function profiler
$ENV:MODE=4; go run .
```

## UCI Support

In default mode, Barracuda responds to:

- `uci`
- `isready`
- `position startpos ...` and `position fen ...`
- `go depth N` and `go infinite`
- `stop`
- `quit`

## Project Structure

### Configuration

- **`config_variables.go`**: Centralized tuning constants for all search/eval parameters
  - TT size, phase tables, search bounds
  - LMR, null-move, aspiration window, PVS, PV-follow settings
  - Quiescence depth and delta margin, profiling iterations
  - **Single source of truth for engine tuning**

### Core Engine

- `main.go`: UCI loop, mode dispatch, search launch/stop flow
- `search.go`: minimax, alpha-beta, LMR, null-move pruning, aspiration/PVS, root search
- `quiescence_search.go`: tactical extension at leaves
- `eval.go`: fast evaluator, legacy evaluator, move-ordering scores
- `pst.go`: PST initialization and phase tables
- `pawn_structure.go`: pawn structure evaluation helper
- `handler.go`: killer move table
- `transposition_table.go`: TT entry format and probe/store logic
- `pv_store.go`: PV table, PV line reconstruction, predicted PV-by-hash cache
- `hashing.go`: fast full/incremental position hashing
- `ucihelper.go`: parser for UCI `go` command options
- `misc.go`: shared structs and utility helpers

### Profiling & Utilities

- `profiling.go`, `profiling_cpu_other.go`, `profiling_cpu_windows.go`: profiling harness and CPU timing
- `autoSyntaxGenerator.py`: helper for bitboard mask generation

### Documentation

- `learn.md`: detailed internal design notes and optimization changelog

## Current Focus

Barracuda is in active search-quality and performance tuning, with emphasis on:

- Reducing node count without tactical regressions
- Improving time-management and practical UCI match behavior
- Tightening move ordering and pruning heuristics further

## Past Performance Optimizations:

- Transitioned to array-based TT and dedicated PV table for faster access - _10x less overhead_
- Implemented LMR and null-move pruning to reduce search tree size - _50% fewer nodes at depth 6+_
- Added aspiration windows and root PVS for more efficient search at the root - _70% fewer nodes at depth 7+_
- Optimized move ordering with killer moves, iterative-depth history, and PV-follow heuristics - _Significant reduction in search tree size and improved tactical performance_
- Refined quiescence search with delta pruning and tactical extensions - _5x reduction in quiescence nodes while maintaining tactical accuracy_
- Improved evaluation speed and accuracy with phase-based tuning and pawn structure evaluation - _300 ELO Gain_
- Added detailed profiling and benchmarking modes to identify bottlenecks and guide optimizations - _Better understanding of performance characteristics_
- Implemented efficient incremental hashing for fast position updates - _Used XOR Self inverse to create an incremental hash that can be updated in O(1) time for moves and unmoves_
- Wrote a custom Backend replacing Old `github.com/corentings/chess/v2` Backend for faster move generation and position updates - _Jump from 350k nodes per second to 1.2M nodes per second_

## Special Thanks
- Special Thanks to [Tarrasch GUI](https://www.triplehappy.com/) for providing a portable open source chess GUI under allowing me to distribute Barracuda with a GUI for easier demonstration, releases of Barracuda are bundled with Tarrasch GUI for easy testing and demonstration of Barracuda's capabilities.
