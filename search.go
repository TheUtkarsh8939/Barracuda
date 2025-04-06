package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/corentings/chess"
)

//go:inline
var nodesVisited int = 0
var transpositionTable = make(map[[16]byte]float64) // Global cache
var killerMoveTable = make(map[uint8][]Move)
var lastBestMoves []string = []string{}

func minimax(position *chess.Position, depth uint8, maximizer bool, alpha float64, beta float64, pst map[chess.Color]map[chess.PieceType][64]int) float64 {
	nodesVisited++
	// Temproary Transpostion table disabled
	if val, ok := transpositionTable[position.Hash()]; ok { // If cached, return value
		return val
	}

	if position.Status() != chess.NoMethod {
		eval := float64(EvaluatePos(position, pst))
		transpositionTable[position.Hash()] = eval // Store evaluation
		return eval

	}
	if depth == 0 {
		eval := quiescence_search(position, int(alpha), int(beta), maximizer, 1, pst)
		// eval := EvaluatePos(position, pst)
		transpositionTable[position.Hash()] = float64(eval) // Store evaluation
		return float64(eval)
	}
	moves := position.ValidMoves()
	sort.Slice(moves, func(i, j int) bool {
		return EvaluateMove(moves[i], position, depth) > EvaluateMove(moves[j], position, depth)
	})
	// InsertionSort(moves, func(a, b *chess.Move) bool {
	// 	return EvaluateMove(a, position, depth) > EvaluateMove(b, position, depth) // Sort best moves first
	// })
	if maximizer {
		bestScore := math.Inf(-1)
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			score := 0.0
			if len(moves)/2 < i && depth >= 3 {
				score = minimax(position.Update(moves[i]), depth-2, !maximizer, alpha, beta, pst)
				if score > bestScore {
					score := minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
					if score > bestScore {
						bestMove = moves[i]
						bestScore = score
					}
				}
			} else {
				score = minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
				if score > bestScore {
					bestMove = moves[i]
					bestScore = score
				}
			}
			alpha = math.Max(alpha, score)
			if beta <= alpha {
				break
			}
		}
		storeKillerMove(depth, Move{bestMove.S1(), bestMove.S2()})

		return bestScore
	} else {
		bestScore := math.Inf(1)
		bestMove := &chess.Move{}
		for i := 0; i < len(moves); i++ {
			score := 0.0
			if len(moves)/2 < i && depth >= 3 {
				score = minimax(position.Update(moves[i]), depth-2, !maximizer, alpha, beta, pst)
				if score < bestScore {
					score := minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
					if score < bestScore {
						bestMove = moves[i]
						bestScore = score
					}
				}
			} else {
				score = minimax(position.Update(moves[i]), depth-1, !maximizer, alpha, beta, pst)
				if score < bestScore {
					bestMove = moves[i]
					bestScore = score
				}
			}
			beta = math.Min(beta, score)

			if beta <= alpha {
				break
			}
		}
		storeKillerMove(depth, Move{bestMove.S1(), bestMove.S2()})
		return bestScore
	}

}

func rateAllMoves(position *chess.Position, depth uint8, pst map[chess.Color]map[chess.PieceType][64]int, isWhite bool) (*chess.Move, float64) {
	bestMove := &chess.Move{}
	bestScore := math.Inf(-1)
	if !isWhite {
		bestScore = math.Inf(1)
	}

	moves := position.ValidMoves()
	transpositionTable = make(map[[16]byte]float64)

	for _, move := range moves {

		score := minimax(position.Update(move), depth-1, !isWhite, math.Inf(-1), math.Inf(1), pst)
		if isCastlingMove(move) {
			score += 200
		}
		if (isWhite && (score > bestScore)) || (!isWhite && (score < bestScore)) {
			bestScore = score
			bestMove = move

		}

		// Restore game properly

		// fmt.Println("Current Best Move:", bestMove, "Score:", bestScore, "Last Searched:", move, "Score of", score)
	}

	return bestMove, bestScore
}
func iterativeDeepening(position *chess.Position, maxDepth uint8, pst map[chess.Color]map[chess.PieceType][64]int, isWhite bool) {

	bestMove := &chess.Move{}
	bestScore := 0.0
	for i := 0; i < int(maxDepth); i++ {
		select {
		case <-stopSearch:
			fmt.Println("bestmove", bestMove)
			return
		default:
			bestMove, bestScore = rateAllMoves(position, uint8(i+1), pst, isWhite)
			lastBestMoves = append(lastBestMoves, bestMove.String())
			fmt.Printf("info depth %d score cp %f \n", i+1, bestScore)
		}

	}
	fmt.Println("bestmove", bestMove)
	lastBestMoves = []string{}
	killerMoveTable = resetKillerMoveTable(killerMoveTable)

}
