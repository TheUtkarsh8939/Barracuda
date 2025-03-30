package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/corentings/chess"
)

// //Test Main
// func main() {
// 	// fen, _ := chess.FEN("r6r/p1pk2pp/3b1n2/3p4/N7/4B1PP/PP3P2/R4RK1 w - - 3 19")
// 	game := chess.NewGame()
// 	pst := initPST()
// 	isWhite := true
// 	startTime := time.Now() //Starts the times

// 	// // Iterative Deepening test
// 	// bestMove := iterativeDeepening(game.Position(), 5, pst, isWhite)
// 	// fmt.Println(bestMove, nodesVisited)

// 	// // Speed test (test a function 100000 times and returns the time it took)
// 	// returnTable := [100000]int{}
// 	// for i := 0; i < len(returnTable); i++ {
// 	// 	returnTable[i] = EvaluatePos(game.Position(), pst)
// 	// }
// 	elpased := time.Since(startTime) //Stops the times

// 	// fmt.Println(returnTable)

// 	// Bot vs Bot
// 	for game.Outcome() == chess.NoOutcome {
// 		bestMove := rateAllMoves(game.Position(), 3, pst, isWhite)
// 		game.Move(bestMove)
// 		fmt.Println(game.Position().Board().Draw())
// 		isWhite = !isWhite
// 	}
// 	fmt.Println(game.Outcome(), game.Method(), game.Position().Status())

// 	fmt.Println(elpased) //Result of performance test

// }

func main() {
	isWhite := true
	scanner := bufio.NewScanner(os.Stdin)
	game := chess.NewGame()
	pst := initPST()
	chess.UseNotation(chess.UCINotation{})
	fmt.Println("id name Barracuda")
	fmt.Println("id author Utkarsh Chandel")
	fmt.Println("uciok")
	firstPosCmd := true
	for scanner.Scan() {
		command := scanner.Text()

		if command == "uci" {
			fmt.Println("id name Barracuda")
			fmt.Println("id author Utkarsh Chandel")
			fmt.Println("uciok")
		} else if command == "isready" {
			fmt.Println("readyok")
		} else if strings.HasPrefix(command, "position") {
			tokens := strings.Split(command, " ")
			if len(tokens) > 3 && firstPosCmd {
				isWhite = false
			}
			firstPosCmd = false
			if tokens[1] == "startpos" {
				game = chess.NewGame()
				for i := 3; i < len(tokens); i++ {
					move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
					game.Move(move)
					if err != nil {
						fmt.Println(err)
					}
					fmt.Println(game.Position().Board().Draw())
				}
			} else {
				fen, _ := chess.FEN(tokens[1] + " w kqKQ 0 0 w")
				game = chess.NewGame(fen)
				for i := 3; i < len(tokens); i++ {
					move, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), tokens[i])
					game.Move(move)
					if err != nil {
						fmt.Println(err)
					}
					fmt.Println(game.Position().Board().Draw())
				}
			}
		} else if strings.HasPrefix(command, "go") {

			bestMove := iterativeDeepening(game.Position(), 5, pst, isWhite) // Implement your engine's move search here

			fmt.Println("bestmove", bestMove)
		} else if command == "quit" {
			break
		}
	}
}
func searchParallely() {

}
