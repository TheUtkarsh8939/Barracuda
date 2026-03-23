package main

import (
	"strings"

	"github.com/corentings/chess/v2"
	"github.com/corentings/chess/v2/opening"
)

func initBook() *opening.BookECO {
	book := opening.NewBookECO()
	return book
}

// parsePGNMoves extracts move strings from a PGN string.
// Removes move numbers (like "1.", "2.") and returns just the moves.
func parsePGNMoves(pgn string) []string {
	tokens := strings.Fields(pgn)
	var moves []string

	for _, token := range tokens {
		// Skip move numbers (e.g., "1.", "2.", "...")
		if strings.HasSuffix(token, ".") {
			continue
		}
		// Skip any other non-move tokens (like annotations or comments)
		if token == "" || token == "*" {
			continue
		}
		moves = append(moves, token)
	}

	return moves
}

func buildLegacyMovesFromUCI(uciMoves []string) ([]*chess.Move, error) {
	game := chess.NewGame()
	legacyMoves := make([]*chess.Move, 0, len(uciMoves))
	for _, token := range uciMoves {
		mv, err := chess.Notation.Decode(chess.UCINotation{}, game.Position(), token)
		if err != nil {
			return nil, err
		}
		legacyMoves = append(legacyMoves, mv)
		if err := game.Move(mv, &chess.PushMoveOptions{}); err != nil {
			return nil, err
		}
	}
	return legacyMoves, nil
}

func findNextMove(uciMoves []string, book *opening.BookECO) string {
	moves, err := buildLegacyMovesFromUCI(uciMoves)
	if err != nil {
		return ""
	}

	openings := book.Possible(moves)
	if len(openings) == 0 {
		return ""
	}

	// Loop through all matching openings to find the next move
	for _, openingLine := range openings {
		// Get the PGN and parse it to extract moves
		pgn := openingLine.PGN()
		pgnMoves := parsePGNMoves(pgn)

		// The next move is at index len(moves)
		if len(pgnMoves) > len(moves) {
			// Convert PGN move notation to chess.Move
			// We need a position to decode the notation
			game := chess.NewGame()
			for i := 0; i < len(moves); i++ {
				game.Move(moves[i], &chess.PushMoveOptions{})
			}

			// Decode the next move in PGN notation
			nextMoveStr := pgnMoves[len(moves)]
			moveObj, err := chess.AlgebraicNotation{}.Decode(game.Position(), nextMoveStr)
			if err == nil {
				return moveObj.String()
			}
		}
	}

	// No valid next move found in any opening
	return ""
}
