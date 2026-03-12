package main

import (
"bufio"
"fmt"
"os"
"strings"
"time"

"github.com/corentings/chess"
)

var stopSearch = make(chan bool)

func main() {
if os.Getenv("BENCH") == "1" {
game := chess.NewGame()
pst := initPST()
isWhite := true
chess.UseNotation(chess.UCINotation{})

nodesVisited = 0
transpositionTable = make(map[[16]byte]ttEntry)
killerMoveTable = make(map[uint8][]Move)
lastBestMoves = make(map[Move]bool)

startTime := time.Now()
iterativeDeepening(game.Position(), 5, pst, isWhite)
elapsed := time.Since(startTime)
fmt.Printf("BENCH: nodes=%d time=%v\n", nodesVisited, elapsed)
return
}

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
options := parseGoCmd(command)
go iterativeDeepening(game.Position(), options.depth, pst, isWhite)
} else if command == "quit" {
stopSearch <- true
return
} else if command == "stop" {
stopSearch <- true
}
}
}
