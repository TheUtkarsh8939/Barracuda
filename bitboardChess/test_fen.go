package bitboardchess

import (
	"fmt"
)

func TestFENManually() {
	// Test the NewBoardFromFEN function
	fen := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	board := NewBoardFromFEN(fen)

	if board == nil {
		fmt.Println("ERROR: NewBoardFromFEN returned nil")
		return
	}

	fmt.Println("✓ Board created successfully")
	fmt.Printf("Turn (true=white): %v\n", board.turn)
	fmt.Printf("CastleRights: 0x%02x\n", board.castleRights)
	fmt.Printf("EnPassant: %d\n", board.enPassant)
	fmt.Printf("HalfmoveCount: %d\n", board.halfmoveCount)
	fmt.Printf("FullmoveNumber: %d\n", board.fullmoveNumber)

	// Verify white pawns
	fmt.Printf("WhitePawns: 0x%016x\n", board.WhitePawns)
	if board.WhitePawns == 0xFF00 {
		fmt.Println("✓ White pawns correct")
	} else {
		fmt.Println("✗ White pawns incorrect")
	}

	// Test invalid FEN
	invalidBoard := NewBoardFromFEN("invalid")
	if invalidBoard == nil {
		fmt.Println("✓ Invalid FEN correctly returns nil")
	} else {
		fmt.Println("✗ Invalid FEN should return nil")
	}
}
