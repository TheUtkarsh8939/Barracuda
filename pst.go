package main

import "github.com/corentings/chess"

// mirrorBoard flips a piece-square table vertically so that Black's PST
// is the mirror image of White's. This ensures both sides use equivalent
// positional incentives relative to their own starting side of the board.
// For example, White's pawn advancement bonus toward rank 8 becomes
// Black's pawn advancement bonus toward rank 1.
func mirrorBoard(pst [64]int) [64]int {
	var mirrored [64]int
	for i := 0; i < 64; i++ {
		rank := i / 8
		file := i % 8
		mirrored[i] = pst[(7-rank)*8+file] // Reflect rank, keep file the same
	}
	return mirrored
}

// initPST initializes piece-square tables as a [3][7][64]int array for fast access.
// Indexed by [Color][PieceType][Square] where Color: 1=White, 2=Black and
// PieceType: 1=King, 2=Queen, 3=Rook, 4=Bishop, 5=Knight, 6=Pawn.
// Using arrays instead of nested maps eliminates map hashing overhead in the hot path.
func initPST() [3][7][64]int {
	var tables [3][7][64]int

	// Black PST definitions
	tables[chess.Black][chess.Pawn] = [64]int{
		0, 0, 0, 0, 0, 0, 0, 0,
		30, 0, 0, 0, 0, 0, 0, 30,
		20, 0, 0, 0, 0, 0, 0, 20,
		10, 0, 0, 0, 0, 0, 0, 10,
		-5, 0, 0, 20, 20, 0, 0, -5,
		-5, 5, 10, 0, 0, 10, 5, -5,
		0, 5, 0, 0, 0, 0, 5, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	tables[chess.Black][chess.Knight] = [64]int{
		-50, -40, -30, -30, -30, -30, -40, -50,
		-40, -20, 20, 5, 5, 20, -20, -40,
		-30, 5, 10, 15, 15, 10, 5, -30,
		-30, 0, 15, 20, 20, 15, 0, -30,
		-30, 5, 15, 20, 20, 15, 5, -30,
		-30, 0, 20, 15, 15, 20, 0, -30,
		-40, -20, 0, 0, 0, 0, -20, -40,
		-50, -40, -30, -30, -30, -30, -40, -50,
	}
	tables[chess.Black][chess.Bishop] = [64]int{
		-20, -10, -10, -10, -10, -10, -10, -20,
		-10, 5, 0, 0, 0, 0, 5, -10,
		-10, 10, 10, 10, 10, 10, 10, -10,
		-10, 0, 10, 10, 10, 10, 0, -10,
		-10, 5, 10, 10, 10, 10, 5, -10,
		-10, 0, 5, 10, 10, 5, 0, -10,
		-10, 10, 0, 0, 0, 0, 10, -10,
		-20, -10, -10, -10, -10, -10, -10, -20,
	}
	tables[chess.Black][chess.Rook] = [64]int{
		0, 0, 0, 5, 5, 0, 0, 0,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		-5, 0, 0, 0, 0, 0, 0, -5,
		5, 10, 10, 10, 10, 10, 10, 5,
		5, 0, 0, 0, 0, 0, 0, 5,
	}
	tables[chess.Black][chess.Queen] = [64]int{
		-20, -10, -10, -5, -5, -10, -10, -20,
		-10, 0, 0, 0, 0, 0, 0, -10,
		-10, 0, 5, 5, 5, 5, 0, -10,
		-5, 0, 5, 5, 5, 5, 0, -5,
		0, 0, 5, 5, 5, 5, 0, -5,
		-10, 5, 5, 5, 5, 5, 0, -10,
		-10, 0, 5, 0, 0, 0, 0, -10,
		-20, -10, -10, -5, -5, -10, -10, -20,
	}
	tables[chess.Black][chess.King] = [64]int{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
	}

	// Generate White's PST by mirroring Black's PST
	pieceTypes := [6]chess.PieceType{chess.King, chess.Queen, chess.Rook, chess.Bishop, chess.Knight, chess.Pawn}
	for _, pt := range pieceTypes {
		tables[chess.White][pt] = mirrorBoard(tables[chess.Black][pt])
	}

	return tables
}
