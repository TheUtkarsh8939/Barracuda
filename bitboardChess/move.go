package bitboardchess

type Move struct {
	From        uint8 // The square index (0-63) from which the piece is moving
	To          uint8 // The square index (0-63) to which the piece is moving
	PieceType   uint8 // The type of piece being moved (e.g., pawn, knight, bishop, rook, queen, king)
	isCapture   bool  // Indicates if the move is a capture move
	isCheck     bool  // Indicates if the move results in a check
	isEnPassant bool  // Indicates if the move is an en passant capture
	isCastle    bool  // Indicates if the move is a castle move
	promoteTo   uint8 // The piece type to promote to (0 if not a promotion)
}

// squareToAlgebraic converts a square index (0-63) to algebraic notation (e.g., 0 -> "a1", 63 -> "h8")
func squareToAlgebraic(square uint8) string {
	files := "abcdefgh"
	ranks := "12345678"
	file := files[square%8]
	rank := ranks[square/8]
	return string(file) + string(rank)
}

// Returns true if the move is a castle move, false otherwise
func (m Move) IsCastleMove() bool {
	return m.isCastle
}

// Returns true if the move is an en passant move, false otherwise
func (m Move) IsEnPassantMove() bool {
	return m.isEnPassant
}

// Returns true if the move is a promotion move, false otherwise
func (m Move) IsPromotion() bool {
	return m.promoteTo != 0
}

// Returns true if the move is a capture move, false otherwise
func (m Move) IsCaptureMove() bool {
	return m.isCapture
}

// Returns true if the move is a check move, false otherwise
func (m Move) IsCheckMove() bool {
	return m.isCheck
}

// Converts the move to a string in algebraic notation (e.g., "e2e4")
func (m Move) String() string {
	return squareToAlgebraic(m.From) + squareToAlgebraic(m.To)
}
