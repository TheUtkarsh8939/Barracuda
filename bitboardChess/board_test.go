package bitboardchess

import (
	"testing"
)

func TestNewBoardFromFENStartingPosition(t *testing.T) {
	fen := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	b := NewBoardFromFEN(fen)

	if b == nil {
		t.Fatal("NewBoardFromFEN returned nil for valid FEN")
	}

	// Verify white pieces are in correct starting positions
	if b.WhitePawns != 0xFF00 {
		t.Errorf("WhitePawns expected 0xFF00, got 0x%016x", b.WhitePawns)
	}

	// Verify black pieces
	if b.BlackPawns != 0x00FF_0000_0000_0000 {
		t.Errorf("BlackPawns expected 0x00FF000000000000, got 0x%016x", b.BlackPawns)
	}

	// Verify turn is white
	if !b.turn {
		t.Errorf("Expected turn=true (white), got false")
	}

	// Verify castle rights: all set (KQkq)
	if b.castleRights != 0x0F {
		t.Errorf("Expected castleRights=0x0F, got 0x%02x", b.castleRights)
	}

	// Verify no en passant
	if b.enPassant != 64 {
		t.Errorf("Expected enPassant=64 (none), got %d", b.enPassant)
	}

	// Verify halfmove count is 0
	if b.halfmoveCount != 0 {
		t.Errorf("Expected halfmoveCount=0, got %d", b.halfmoveCount)
	}
}

func TestNewBoardFromFENWithEnPassant(t *testing.T) {
	fen := "rnbqkbnr/pp1ppppp/8/2pP4/8/8/PPP1PPPP/RNBQKBNR w KQkq c6 0 3"
	b := NewBoardFromFEN(fen)

	if b == nil {
		t.Fatal("NewBoardFromFEN returned nil for valid FEN")
	}

	// c6 should be square 42 (file=2, rank=5)
	if b.enPassant != 42 {
		t.Errorf("Expected enPassant=42 (c6), got %d", b.enPassant)
	}

	if b.turn != true {
		t.Errorf("Expected turn=true (white), got false")
	}

	if b.halfmoveCount != 0 {
		t.Errorf("Expected halfmoveCount=0, got %d", b.halfmoveCount)
	}
}

func TestNewBoardFromFENBlackToMove(t *testing.T) {
	fen := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"
	b := NewBoardFromFEN(fen)

	if b == nil {
		t.Fatal("NewBoardFromFEN returned nil for valid FEN")
	}

	if b.turn != false {
		t.Errorf("Expected turn=false (black), got true")
	}

	// e3 should be square 20 (file=4, rank=2)
	if b.enPassant != 20 {
		t.Errorf("Expected enPassant=20 (e3), got %d", b.enPassant)
	}
}

func TestNewBoardFromFENPartialCastling(t *testing.T) {
	fen := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w Kq - 0 1"
	b := NewBoardFromFEN(fen)

	if b == nil {
		t.Fatal("NewBoardFromFEN returned nil for valid FEN")
	}

	// Should have only K and q rights (bits 0 and 3)
	expected := uint8((1 << 0) | (1 << 3))
	if b.castleRights != expected {
		t.Errorf("Expected castleRights=0x%02x, got 0x%02x", expected, b.castleRights)
	}
}

func TestNewBoardFromFENNoCastling(t *testing.T) {
	fen := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w - - 0 1"
	b := NewBoardFromFEN(fen)

	if b == nil {
		t.Fatal("NewBoardFromFEN returned nil for valid FEN")
	}

	if b.castleRights != 0 {
		t.Errorf("Expected castleRights=0, got 0x%02x", b.castleRights)
	}
}

func TestNewBoardFromFENInvalidFEN(t *testing.T) {
	fen := "invalid fen string"
	b := NewBoardFromFEN(fen)

	if b != nil {
		t.Errorf("Expected nil for invalid FEN, got %v", b)
	}
}
