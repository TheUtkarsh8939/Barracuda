# Barracuda

Barracuda is a UCI chess engine completely written in Golang, The engine is both UCI compiliant and also has a GUI created using Wails, you can find the GUI in the `gui/telegui` directory.

**NOTE: Barracuda is an engine made solely by a single 14 year old, I do not have enough time for extensive testing and optimization, please report the bugs if you find any**

## Heaurastics, Search Techniques and Evaluation implemented

### Search Techniques
- Iterative deepening alpha-beta search
- Quiescence search with stand-pat and delta pruning
- Late Move Reduction (LMR)
- Null-move pruning
- Aspiration windows + root PVS

### Heaurastics and Move Ordering
- Killer moves + iterative-depth move history + PV-follow ordering
- Array-based transposition table and dedicated PV table
- Usage of linear best move search instead of full on sorting in move sorting

### Evaluation
- Material evaluation with piece-square tables
- Bishop and Rook pair bonuses
- Passed Pawns and Pawn Islands
- King Shelter Bonus
- Center Control Bonus
- Rook on open file bonus
- Isolated Pawn Penalty
- Double Pawn Penalty

## Running

### Option 1: Running the engine in UCI mode

- Clone The repository
- For windows, the repo has precompiled binaries from Version 6.1 to Version 7. You can find them in the `bin` directory, load the binary in any Chess GUI and you are good to go

### Option 2: Running the engine with the GUI

- Go to releases and download the latest release, the release contains precompiled binaries for both the engine and the GUI.
- Extract the zip file and run `GUI.exe`. The GUI will open and a terminal window will open in the background
- DO NOT CLOSE THE TERMINAL WINDOW, it is required for the GUI to work, the terminal window is where the engine is running and communicating with the GUI, if you close it, the engine will stop and the GUI will stop working as well.

### Configuration

- **`config_variables.go`**: Centralized tuning constants for all search/eval parameters
  - TT size, phase tables, search bounds
  - LMR, null-move, aspiration window, PVS, PV-follow settings
  - Quiescence depth and delta margin, profiling iterations
  - **Single source of truth for engine tuning**

## Docs
- For more details there is a 25 page documentation about the engine in `learn.md`


