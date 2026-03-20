package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/corentings/chess/v2"
)

// nodesVisited counts total nodes evaluated during a search — useful for benchmarking speed.
var nodesVisited int = 0
var leafNodesVisited int = 0       // Separate count for leaf nodes (quiescence and terminal positions) to analyze performance bottlenecks.
var quiescenceNodesVisited int = 0 // Count of nodes visited specifically in quiescence search, to evaluate its contribution to total search time.
var evaluateFunctionCalls int = 0  // Count of how many times the static evaluation function is called, to analyze its impact on performance.
// main is the entry point of the program. It initializes the UCI protocol loop for communication
// with chess GUIs and handles commands such as "uci", "isready", "position", and "go".
// The commented-out section above demonstrates a test harness for benchmarking the search logic.

var stopSearch = make(chan bool) // Channel to signal stopping the search process.
func main() {
	// Benchmark mode: run a depth-7 search from the starting position and print timing info.
	// Usage: BENCH=1 ./barracuda
	if os.Getenv("MODE") == "1" {
		game := chess.NewGame()
		pst := initPST()
		isWhite := true

		nodesVisited = 0
		clearTT()
		clearKillerTable()
		lastBestMoves = make(map[Move]bool)

		startTime := time.Now()
		iterativeDeepening(game.Position(), 7, &pst, isWhite)
		elapsed := time.Since(startTime)
		fmt.Printf("BENCH: nodes=%d, leafNodes=%d, quiescenceNodes=%d, evaluationDone=%d, positionUpdateCalls=%d, time=%v\n", nodesVisited, leafNodesVisited, quiescenceNodesVisited, evaluateFunctionCalls, positionUpdateCalls, elapsed)
		return
	} else if os.Getenv("MODE") == "2" {
		// rng := rand.New(rand.NewSource(42))
		pst := initPST()
		fen, _ := chess.FEN("8/pp2ppkp/2n2q2/2b1p3/2B1P3/2N5/PPP2PPP/2KR2R1 b - - 3 18")
		testGame := chess.NewGame(fen)
		eval1 := LegacyEvaluatePos(testGame.Position(), &pst)
		fmt.Printf("Evaluation: %d\n", eval1)
		eval2 := EvaluatePos(testGame.Position(), &pst)
		fmt.Printf("Fast Evaluation: %d\n", eval2)
		return
	} else if os.Getenv("MODE") == "3" {
		fmt.Println("Benchmarking")
		fen, _ := chess.FEN("1rbqkbnr/pppppppp/8/8/1n1P4/2N1P3/PPP1NPPP/R1BQKB1R b KQk d3 0 4")
		testGame := chess.NewGame(fen)
		pst := initPST()
		// vm := testGame.Position().ValidMoves()
		pstptr := &pst
		startTime := time.Now()
		for i := 0; i < 10; i++ {
			//Test Eval speed by evaluating the same position 1 million times.
			// sinkErr = testGame.UnsafeMove(&vm[0], nil)
			EvaluatePos(testGame.Position(), pstptr)
		}
		elapsedTime := time.Since(startTime)

		fmt.Printf("BENCH: time=%v\n", elapsedTime)
		fmt.Printf("BENCH: timesFunctionRan=%d\n", 1000000)
		fmt.Printf("BENCH: evaluationDone=%d\n", evaluateFunctionCalls)
		return
	} else if os.Getenv("MODE") == "4" {
		Benchmark()
		return
	}

	// Indicates whether the engine is playing as White.
	isWhite := true

	// Scanner to read input commands from the standard input (UCI protocol commands).
	scanner := bufio.NewScanner(os.Stdin)

	// Initialize a new chess game and the piece-square table (PST) for evaluation.
	game := chess.NewGame()
	pst := initPST()

	// Print engine identification information.
	fmt.Println("id name Barracuda")
	fmt.Println("id author Utkarsh Chandel")
	fmt.Println("uciok")

	firstPosCmd := true // Tracks if the "position" command is the first one received.
	//Move list for opening book(not implemented yet)
	moveList := []string{}

	// Main loop to process UCI commands.
	for scanner.Scan() {
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
			// Handle the "position" command to set up the board state.
			tokens := strings.Split(command, " ")
			fmt.Println(tokens)
			if len(tokens) > 3 && firstPosCmd {
				isWhite = false
			}
			firstPosCmd = false

			if tokens[1] == "startpos" {
				// Set up the board to the starting position.
				game = chess.NewGame()
				for i := 3; i < len(tokens); i++ {
					move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
					game.Move(move, &chess.PushMoveOptions{}) // Skip move validation for speed; assumes input is correct.
					if err != nil {
						fmt.Println(err)

					}
					moveList = append(moveList, tokens[i])
					// fmt.Println(game.Position().Board().Draw())
				}
				if len(moveList)%2 != 0 {
					isWhite = false
				}
			} else {
				// Set up the board to a custom position using FEN notation.
				fenString := tokens[2] + " " + tokens[3] + " " + tokens[4] + " " + tokens[5] + " " + tokens[6] + " " + tokens[7]
				fen, err := chess.FEN(fenString)
				if err != nil {
					fmt.Println("Invalid FEN string:", err)
					continue
				}
				game = chess.NewGame(fen)
				for i := 9; i < len(tokens); i++ {
					moveList = append(moveList, tokens[i])
				}
				// fen, _ := chess.FEN(tokens[2] + " " + tokens[3] + " " + tokens[4] + " " + tokens[5] + " " + tokens[6] + " " + tokens[7])
				// game = chess.NewGame(fen)
				// for i := 3; i < len(tokens); i++ {
				// 	move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
				// 	game.Move(move)
				// 	if err != nil {
				// 		fmt.Println(err)
				// 	}
				// 	fmt.Println(game.Position().Board().Draw())
				// }
			}
		} else if strings.HasPrefix(command, "go") {
			// Handle the "go" command to start the search for the best move.
			options := parseGoCmd(command)
			go iterativeDeepening(game.Position(), options.depth, &pst, isWhite) // Start the search process.
		} else if command == "quit" {
			// Handle the "quit" command to exit the program.
			stopSearch <- true
			return
		} else if command == "stop" {
			// Handle the "stop" command to halt the search process.
			stopSearch <- true
		} else if command == "debug" {
			//Custom Instructions LOL
			fen, _ := chess.FEN("1rbqkbnr/pppppppp/8/8/1n1P4/2N1P3/PPP1NPPP/R1BQKB1R b KQk d3 0 4")
			testGame := chess.NewGame(fen)

			fmt.Println(EvaluatePos(testGame.Position(), &pst))
			fmt.Println(testGame.Position().Board().Draw())
		}
	}
}
