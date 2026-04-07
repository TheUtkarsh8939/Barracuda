package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/corentings/chess/v2"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx                            context.Context
	game                           *chess.Game
	squareMap                      map[int]chess.Piece
	vmUci                          []string
	movesPlayed                    []string
	engine                         *EngineRunner
	bestMove                       string
	bestMoveMu                     sync.RWMutex
	bestMoveCh                     chan string
	bestMoveChMu                   sync.Mutex
	timeRemainingWhiteCentiSeconds int
	timeRemainingBlackCentiSeconds int
	turnStartedAt                  time.Time
}

// NewApp creates a new App application struct
func NewApp(game *chess.Game, squareMap map[int]chess.Piece, vmUci []string, movesPlayed []string) *App {
	return &App{game: game, squareMap: squareMap, vmUci: vmUci, movesPlayed: movesPlayed, timeRemainingWhiteCentiSeconds: 60000, timeRemainingBlackCentiSeconds: 12000, turnStartedAt: time.Now()}
}

func centisecondsFromDuration(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(d / (10 * time.Millisecond))
}

func (a *App) consumeWhiteTime(elapsed time.Duration) {
	a.timeRemainingWhiteCentiSeconds -= centisecondsFromDuration(elapsed)
	if a.timeRemainingWhiteCentiSeconds < 0 {
		a.timeRemainingWhiteCentiSeconds = 0
	}
}

func (a *App) consumeBlackTime(elapsed time.Duration) {
	a.timeRemainingBlackCentiSeconds -= centisecondsFromDuration(elapsed)
	if a.timeRemainingBlackCentiSeconds < 0 {
		a.timeRemainingBlackCentiSeconds = 0
	}
}

// Check for mate
func (a *App) IsGameOver() (bool, string) {
	if a.game.Position().Status() == chess.Checkmate {
		return true, "Checkmate"
	}
	if a.game.Position().Status() == chess.Stalemate {
		return true, "Stalemate"
	}
	return false, ""
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	var err error
	a.engine, err = StartEngine()
	if err != nil {
		runtime.LogErrorf(a.ctx, "failed to start engine: %v", err)
		return
	}

	a.engine.SetLineHandler(a.handleEngineLine)
	runtime.EventsOn(a.ctx, "movePlayed", func(payload ...any) {
		if len(payload) > 0 {
			if a.turnStartedAt.IsZero() {
				a.turnStartedAt = time.Now()
			}

			// White's clock runs while waiting for player's move.
			a.consumeWhiteTime(time.Since(a.turnStartedAt))
			if a.timeRemainingWhiteCentiSeconds == 0 {
				runtime.EventsEmit(a.ctx, "gameOver", "Game Over: White flag fell")
				return
			}

			move, ok := payload[0].(string)
			if !ok {
				runtime.LogError(a.ctx, "movePlayed payload is not a string")
				return
			}
			chessMove, err := chess.UCINotation{}.Decode(a.game.Position(), move)
			if err != nil {
				runtime.LogErrorf(a.ctx, "failed to decode UCI move: %v", err)
				return
			}
			if err := a.game.UnsafeMove(chessMove, &chess.PushMoveOptions{}); err != nil {
				runtime.LogErrorf(a.ctx, "failed to apply move: %v", err)
				return
			}

			// White move is now complete; start black's turn timing window.
			a.turnStartedAt = time.Now()

			a.syncBoardState()
			a.EmitSquareMap()
			status, reason := a.IsGameOver()
			if status {
				runtime.EventsEmit(a.ctx, "gameOver", "Game Over: "+reason)
				return
			}
			moves := a.game.Moves()
			uciMoves := make([]string, len(moves))
			for i, move := range moves {
				uciMoves[i] = move.String()
			}

			a.bestMoveChMu.Lock()
			a.bestMoveCh = make(chan string, 1)
			waitCh := a.bestMoveCh
			a.bestMoveChMu.Unlock()

			postionCmd := "position startpos"
			if len(uciMoves) > 0 {
				postionCmd = fmt.Sprintf("position startpos moves %s", strings.Join(uciMoves, " "))
			}
			if err := a.engine.Send(postionCmd); err != nil {
				runtime.LogErrorf(a.ctx, "failed to send position command: %v", err)
				return
			}

			engineTurnStartedAt := time.Now()
			goCmd := fmt.Sprintf("go wtime %d btime %d", a.timeRemainingWhiteCentiSeconds*10, a.timeRemainingBlackCentiSeconds*10)
			if err := a.engine.Send(goCmd); err != nil {
				runtime.LogErrorf(a.ctx, "failed to send go command: %v", err)
				return
			}

			var bestMove string
			select {
			case bm := <-waitCh:
				bestMove = bm
				runtime.EventsEmit(a.ctx, "engine:bestmove", bm)
			case <-time.After(5 * time.Second):
				runtime.LogWarning(a.ctx, "timed out waiting for bestmove")
				return
			}

			// Black's clock runs while engine is thinking.
			a.consumeBlackTime(time.Since(engineTurnStartedAt))
			if a.timeRemainingBlackCentiSeconds == 0 {
				runtime.EventsEmit(a.ctx, "gameOver", "Game Over: Black flag fell")
				return
			}

			a.bestMoveChMu.Lock()
			if a.bestMoveCh == waitCh {
				a.bestMoveCh = nil
			}
			a.bestMoveChMu.Unlock()

			nMove, err := chess.UCINotation{}.Decode(a.game.Position(), bestMove)
			if err != nil {
				runtime.LogErrorf(a.ctx, "failed to decode best move: %v", err)
				return
			}
			if err := a.game.UnsafeMove(nMove, &chess.PushMoveOptions{}); err != nil {
				runtime.LogErrorf(a.ctx, "failed to apply best move: %v", err)
				return
			}

			a.syncBoardState()
			a.EmitSquareMap()
			status, reason = a.IsGameOver()
			if status {
				runtime.EventsEmit(a.ctx, "gameOver", reason)
				return
			}

			a.turnStartedAt = time.Now()
		}

	})
}

func (a *App) handleEngineLine(line string) {
	if !strings.HasPrefix(line, "bestmove ") {
		return
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return
	}

	a.bestMoveMu.Lock()
	a.bestMove = fields[1]
	a.bestMoveMu.Unlock()

	a.bestMoveChMu.Lock()
	if a.bestMoveCh != nil {
		select {
		case a.bestMoveCh <- fields[1]:
		default:
		}
	}
	a.bestMoveChMu.Unlock()
}

func (a *App) LatestBestMove() string {
	a.bestMoveMu.RLock()
	defer a.bestMoveMu.RUnlock()
	return a.bestMove
}

// domReady is called when the frontend is loaded and ready to receive events.
func (a *App) domReady(ctx context.Context) {
	a.ctx = ctx
	a.turnStartedAt = time.Now()
	a.EmitSquareMap()
}

type boardStatePayload struct {
	SquareMap   map[int]chess.Piece `json:"squareMap"`
	VmUci       []string            `json:"vmUci"`
	MovesPlayed []string            `json:"movesPlayed"`
}

func (a *App) syncBoardState() {
	a.squareMap = convertSquareMapToA8H1Index(a.game.Position().Board().SquareMap())

	validMoves := a.game.ValidMoves()
	a.vmUci = make([]string, len(validMoves))
	for i, move := range validMoves {
		a.vmUci[i] = move.String()
	}

	moves := a.game.Moves()
	positions := a.game.Positions()
	a.movesPlayed = make([]string, len(moves))
	algebraicNotation := chess.AlgebraicNotation{}
	for i, move := range moves {
		if i < len(positions) {
			a.movesPlayed[i] = algebraicNotation.Encode(positions[i], move)
			continue
		}
		a.movesPlayed[i] = move.String()
	}
}

// EmitSquareMap marshals board state and sends it to the frontend.
func (a *App) EmitSquareMap() {
	payloadJSON, err := json.Marshal(boardStatePayload{
		SquareMap:   a.squareMap,
		VmUci:       a.vmUci,
		MovesPlayed: a.movesPlayed,
	})
	if err != nil {
		runtime.LogErrorf(a.ctx, "failed to marshal board state: %v", err)
		return
	}

	runtime.EventsEmit(a.ctx, "squareMap:updated", string(payloadJSON))
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}
