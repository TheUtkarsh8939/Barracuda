package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

// Global counters used by benchmark modes and UCI info output.
var nodesVisited int = 0
var leafNodesVisited int = 0       // Separate count for leaf nodes (quiescence and terminal positions) to analyze performance bottlenecks.
var quiescenceNodesVisited int = 0 // Count of nodes visited specifically in quiescence search, to evaluate its contribution to total search time.
var evaluateFunctionCalls int = 0  // Count of how many times the static evaluation function is called, to analyze its impact on performance.
var aspirationResearches int = 0   // Count of how many times the search had to be restarted due to aspiration window failures, to evaluate the effectiveness of the aspiration window settings.
var lmrResearches int = 0          // Count of how many times moves were reduced by LMR and then had to be re-searched at full depth, to analyze the effectiveness of LMR settings.
var nullMovePrunes int = 0         // Count of how many times null-move pruning successfully pruned a node, to evaluate the effectiveness of null-move pruning settings.

var moveArray []string
var stopSearch = make(chan bool, 1) // Buffered to avoid deadlocks when no search goroutine is waiting.

func clearStopSignals() {
	for {
		select {
		case <-stopSearch:
		default:
			return
		}
	}
}

func requestStopSearch() {
	select {
	case stopSearch <- true:
	default:
	}
}

func applyMovesUCI(game *chess.Game, moveTokens []string) error {
	for _, token := range moveTokens {
		move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), token)
		if err != nil {
			return err
		}
		if err := game.Move(move, &chess.PushMoveOptions{}); err != nil {
			return err
		}
		moveArray = append(moveArray, token)
	}
	return nil
}

func setPositionFromCommand(game **chess.Game, command string) error {
	tokens := strings.Fields(command)
	if len(tokens) < 2 {
		return fmt.Errorf("invalid position command")
	}

	if tokens[1] == "startpos" {
		g := chess.NewGame()
		moveArray = nil
		movesIdx := -1
		for i := 2; i < len(tokens); i++ {
			if tokens[i] == "moves" {
				movesIdx = i
				break
			}
		}
		if movesIdx != -1 {
			if err := applyMovesUCI(g, tokens[movesIdx+1:]); err != nil {
				return err
			}
		}
		*game = g
		return nil
	}

	if tokens[1] == "fen" {
		if len(tokens) < 8 {
			return fmt.Errorf("invalid FEN in position command")
		}
		fenString := strings.Join(tokens[2:8], " ")
		fen, err := chess.FEN(fenString)
		if err != nil {
			return err
		}
		g := chess.NewGame(fen)
		moveArray = nil
		movesIdx := -1
		for i := 8; i < len(tokens); i++ {
			if tokens[i] == "moves" {
				movesIdx = i
				break
			}
		}
		if movesIdx != -1 {
			if err := applyMovesUCI(g, tokens[movesIdx+1:]); err != nil {
				return err
			}
		}
		*game = g
		return nil
	}

	return fmt.Errorf("unsupported position command")
}

func main() {
	// MODE controls startup behavior:
	//   MODE=1 -> depth-7 search benchmark from start position
	//   MODE=2 -> eval parity/debug output on a fixed FEN
	//   MODE=3 -> evaluation micro-benchmark
	//   MODE=4 -> hot-function profiling harness
	//   unset  -> normal UCI loop
	if os.Getenv("MODE") == "1" {
		game := chess.NewGame()
		pst := initPST()
		isWhite := true

		nodesVisited = 0
		clearTT()
		clearKillerTable()
		lastBestMoves = make(map[Move]bool)

		startTime := time.Now()
		iterativeDeepening(game.Position(), 9, &pst, isWhite)
		elapsed := time.Since(startTime)
		fmt.Printf("BENCH: nodes=%d, leafNodes=%d, quiescenceNodes=%d, evaluationDone=%d, \npositionUpdateCalls=%d, LMRresearches=%d, aspirationResearches=%d, nullMovePrunes=%d time=%v\n", nodesVisited, leafNodesVisited, quiescenceNodesVisited, evaluateFunctionCalls, positionUpdateCalls, lmrResearches, aspirationResearches, nullMovePrunes, elapsed)
		return
	} else if os.Getenv("MODE") == "2" {
		game := chess.NewGame()
		book := initBook()
		applyMovesUCI(game, []string{"e2e4", "e7e5"})
		movesPlayed := game.Moves()
		movesInSan, _ := movesToAlgebraicNotation(movesPlayed)
		nextMove := findNextMove(movesInSan, book)
		nextMoveInUci, _ := moveFromAlgebraicToUCI(game.Position(), nextMove)
		fmt.Printf("Next move in book is: %s\n", nextMoveInUci)
		//TEST OPENING BOOK
		// game := chess.NewGame()
		// applyMovesUCI(game, []string{"c2c4"})
		// book := initBook()
		// nextMove := findNextMove([]string{}, book)
		// fmt.Printf("Next move in book is: %s\n", nextMove)
		return
	} else if os.Getenv("MODE") == "3" {
		// *UNUSED
		return
	} else if os.Getenv("MODE") == "4" {
		Benchmark()
		return
	}

	// UCI mode: read commands from stdin.
	scanner := bufio.NewScanner(os.Stdin)

	// Initialize a new chess game and the piece-square table (PST) for evaluation.
	game := chess.NewGame()
	pst := initPST()
	book := initBook() // Initialize the opening book once at startup, since it can be reused across multiple searches and is expensive to load.
	// Print engine identification information.
	fmt.Println("id name Barracuda")
	fmt.Println("id author Utkarsh Chandel")
	fmt.Println("uciok")

	searching := false
	searchDone := make(chan struct{}, 1)

	// Main loop to process UCI commands.
	for scanner.Scan() {
		select {
		case <-searchDone:
			searching = false
		default:
		}

		command := scanner.Text()

		if command == "uci" {
			// Respond to the "uci" command with engine identification.
			fmt.Println("id name Barracuda")
			fmt.Println("id author Utkarsh Chandel")
			fmt.Println("uciok")
		} else if command == "isready" {
			// Respond to the "isready" command to indicate readiness.
			fmt.Println("readyok")
		} else if strings.HasPrefix(command, "position") {
			if searching {
				requestStopSearch()
			}
			if err := setPositionFromCommand(&game, command); err != nil {
				fmt.Println("info string position parse error:", err)
			}
		} else if strings.HasPrefix(command, "go") {
			if searching {
				requestStopSearch()
			}
			clearStopSignals()
			isWhite := game.Position().Turn() == chess.White
			options := parseGoCmd(command)
			searching = true
			go func(pos *chess.Position, depth uint8, sideIsWhite bool) {
				if len(moveArray) < 8 {
					moves := game.Moves()
					sanMoves, err := movesToAlgebraicNotation(moves)
					if err != nil {
						fmt.Println("info string error converting moves to SAN:", err)
						return
					}
					nextMove := findNextMove(sanMoves, book)
					if nextMove != "" {
						nextMoveUCI, err := moveFromAlgebraicToUCI(pos, nextMove)
						if err != nil {
							fmt.Println("info string error converting next move to UCI:", err)
						} else {
							fmt.Printf("info string found book move: %s\n", nextMoveUCI)
							fmt.Printf("bestmove %s\n", nextMoveUCI)
							return
						}
					}
				}

				iterativeDeepening(pos, depth, &pst, sideIsWhite)
				select {
				case searchDone <- struct{}{}:
				default:
				}
			}(game.Position(), options.depth, isWhite)
		} else if command == "quit" {
			// Handle the "quit" command to exit the program.
			requestStopSearch()
			return
		} else if command == "stop" {
			// Handle the "stop" command to halt the search process.
			requestStopSearch()
		} else if command == "debug" {
			//Custom Instructions LOL
			fmt.Println(moveArray)
		}
	}
}
