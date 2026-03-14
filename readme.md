# NOTE
>The project is now discontinued due to an unidentified bug causing move repetions and weird moves if anybody ever finds it please tell me, the search functions are perfectly usable the issue is in either Evaluation or UCI Implemantion

>I am currently rewrting the engine is C for better performance, this project will be archived

# Barracuda

Barracuda is a Go chess engine built on top of `github.com/corentings/chess`.
It uses minimax with alpha-beta pruning, iterative deepening, quiescence search,
piece-square tables, killer moves, and a depth-aware transposition table.

## Current Status

The engine is actively being tuned and optimized.

- Search and evaluation are implemented.
- Transposition-table reuse across iterative deepening is enabled.
- Move ordering has been optimized significantly.
- The project currently includes a test harness in `main.go` for direct benchmarking.

## Build

```powershell
go build -o Barracuda.exe .
```

## Run

With the current benchmark-style `main.go`:

```powershell
go run .
```

This prints search info, the chosen move, visited nodes, and elapsed time for the
hardcoded test position.

## Main Files

- `main.go` — entry point, currently set up as a local test harness
- `search.go` — minimax, alpha-beta, iterative deepening, transposition table
- `eval.go` — static evaluation and move-ordering heuristics
- `quiescence_search.go` — tactical extension at leaf nodes
- `pst.go` — piece-square tables
- `handler.go` — killer-move bookkeeping
- `ucihelper.go` — parsing for UCI `go` options
- `misc.go` — shared structs and helper utilities

## Strengths

- Much faster than the original version after recent search optimizations
- Reasonable tactical search for a small engine
- Clean enough structure to continue improving

## Next Likely Improvements

- Add an opening book
- Add proper clock-based time management
- Improve transposition table entries with bound flags
- Extend quiescence search and pruning

## Notes

There is a more detailed implementation document in `learn.md`.
