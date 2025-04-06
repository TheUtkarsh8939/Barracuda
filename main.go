package main

import (
	"fmt"
	"time"

	"github.com/corentings/chess"
)

// Test Main
func main() {
	fen, _ := chess.FEN("r1b2rk1/pp1pqppp/2p5/3nP3/1b1Q1P2/2N5/PPPBB1PP/R3K2R b KQ - 2 12")
	game := chess.NewGame(fen)
	pst := initPST()
	isWhite := false
	// vm := game.ValidMoves()
	startTime := time.Now() //Starts the times

	// Iterative Deepening test
	iterativeDeepening(game.Position(), 5, pst, isWhite)
	fmt.Println(nodesVisited)

	// // Speed test (test a function 100000 times and returns the time it took)
	// returnTable := [100]int{}
	// for i := 0; i < len(returnTable); i++ {
	// 	InsertionSort(vm, func(a, b *chess.Move) bool {
	// 		return EvaluateMove(a, game.Position(), 1) > EvaluateMove(b, game.Position(), 2) // Sort best moves first
	// 	})
	// }
	elpased := time.Since(startTime) //Stops the times

	// fmt.Println(returnTable)

	// Bot vs Bot
	// for game.Outcome() == chess.NoOutcome {
	// 	bestMove := rateAllMoves(game.Position(), 3, pst, isWhite)
	// 	game.Move(bestMove)
	// 	fmt.Println(game.Position().Board().Draw())
	// 	isWhite = !isWhite
	// }
	// fmt.Println(game.Outcome(), game.Method(), game.Position().Status())

	fmt.Println(elpased) //Result of performance test

}

var stopSearch = make(chan bool)

// func main() {
// 	isWhite := true
// 	scanner := bufio.NewScanner(os.Stdin)
// 	game := chess.NewGame()
// 	pst := initPST()
// 	chess.UseNotation(chess.UCINotation{})
// 	fmt.Println("id name Barracuda")
// 	fmt.Println("id author Utkarsh Chandel")
// 	fmt.Println("uciok")
// 	firstPosCmd := true
// 	for scanner.Scan() {
// 		command := scanner.Text()

// 		if command == "uci" {
// 			fmt.Println("id name Barracuda")
// 			fmt.Println("id author Utkarsh Chandel")
// 			fmt.Println("uciok")
// 		} else if command == "isready" {
// 			fmt.Println("readyok")
// 		} else if strings.HasPrefix(command, "position") {
// 			tokens := strings.Split(command, " ")
// 			if len(tokens) > 3 && firstPosCmd {
// 				isWhite = false
// 			}
// 			firstPosCmd = false
// 			if tokens[1] == "startpos" {
// 				game = chess.NewGame()
// 				for i := 3; i < len(tokens); i++ {
// 					move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
// 					game.Move(move)
// 					if err != nil {
// 						fmt.Println(err)
// 					}
// 					fmt.Println(game.Position().Board().Draw())
// 				}
// 			} else {
// 				fen, _ := chess.FEN(tokens[1] + " w kqKQ 0 0 w")
// 				game = chess.NewGame(fen)
// 				for i := 3; i < len(tokens); i++ {
// 					move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
// 					game.Move(move)
// 					if err != nil {
// 						fmt.Println(err)
// 					}
// 					fmt.Println(game.Position().Board().Draw())
// 				}
// 			}
// 		} else if strings.HasPrefix(command, "go") {
// 			options := parseGoCmd(command)
// 			go iterativeDeepening(game.Position(), options.depth, pst, isWhite) // Implement your engine's move search here

// 		} else if command == "quit" {
// 			stopSearch <- true
// 			return
// 		} else if command == "stop" {
// 			stopSearch <- true
// 		}
// 	}
// }
