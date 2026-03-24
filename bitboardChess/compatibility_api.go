package bitboardchess

import (
	"errors"
	"fmt"
	"strings"
)

// Compatibility layer to expose a subset of github.com/corentings/chess/v2 style APIs
// backed by this bitboard engine.

type Color uint8

type PieceType uint8

type Piece uint8

type Square uint8

type MoveTag uint8

type Method uint8

type CastleRights string

const (
	NoColor Color = iota
	White
	Black
)

const (
	NoPieceType PieceType = iota
	King
	Queen
	Rook
	Bishop
	Knight
	Pawn
)

const (
	NoPiece Piece = iota
	WhiteKing
	WhiteQueen
	WhiteRook
	WhiteBishop
	WhiteKnight
	WhitePawn
	BlackKing
	BlackQueen
	BlackRook
	BlackBishop
	BlackKnight
	BlackPawn
)

const (
	Capture MoveTag = iota + 1
	Check
	EnPassant
	KingSideCastle
	QueenSideCastle
)

const (
	NoMethod Method = iota
	Checkmate
	Stalemate
)

const (
	NoSquare Square = 64
)

const (
	A1 Square = 0
	B1 Square = 1
	C1 Square = 2
	D1 Square = 3
	E1 Square = 4
	F1 Square = 5
	G1 Square = 6
	H1 Square = 7
	A2 Square = 8
	B2 Square = 9
	C2 Square = 10
	D2 Square = 11
	E2 Square = 12
	F2 Square = 13
	G2 Square = 14
	H2 Square = 15
	A3 Square = 16
	B3 Square = 17
	C3 Square = 18
	D3 Square = 19
	E3 Square = 20
	F3 Square = 21
	G3 Square = 22
	H3 Square = 23
	A4 Square = 24
	B4 Square = 25
	C4 Square = 26
	D4 Square = 27
	E4 Square = 28
	F4 Square = 29
	G4 Square = 30
	H4 Square = 31
	A5 Square = 32
	B5 Square = 33
	C5 Square = 34
	D5 Square = 35
	E5 Square = 36
	F5 Square = 37
	G5 Square = 38
	H5 Square = 39
	A6 Square = 40
	B6 Square = 41
	C6 Square = 42
	D6 Square = 43
	E6 Square = 44
	F6 Square = 45
	G6 Square = 46
	H6 Square = 47
	A7 Square = 48
	B7 Square = 49
	C7 Square = 50
	D7 Square = 51
	E7 Square = 52
	F7 Square = 53
	G7 Square = 54
	H7 Square = 55
	A8 Square = 56
	B8 Square = 57
	C8 Square = 58
	D8 Square = 59
	E8 Square = 60
	F8 Square = 61
	G8 Square = 62
	H8 Square = 63
)

func (p Piece) Type() PieceType {
	switch p {
	case WhiteKing, BlackKing:
		return King
	case WhiteQueen, BlackQueen:
		return Queen
	case WhiteRook, BlackRook:
		return Rook
	case WhiteBishop, BlackBishop:
		return Bishop
	case WhiteKnight, BlackKnight:
		return Knight
	case WhitePawn, BlackPawn:
		return Pawn
	default:
		return NoPieceType
	}
}

func (p Piece) Color() Color {
	switch p {
	case WhiteKing, WhiteQueen, WhiteRook, WhiteBishop, WhiteKnight, WhitePawn:
		return White
	case BlackKing, BlackQueen, BlackRook, BlackBishop, BlackKnight, BlackPawn:
		return Black
	default:
		return NoColor
	}
}

func NewPiece(pt PieceType, color Color) Piece {
	switch color {
	case White:
		switch pt {
		case King:
			return WhiteKing
		case Queen:
			return WhiteQueen
		case Rook:
			return WhiteRook
		case Bishop:
			return WhiteBishop
		case Knight:
			return WhiteKnight
		case Pawn:
			return WhitePawn
		}
	case Black:
		switch pt {
		case King:
			return BlackKing
		case Queen:
			return BlackQueen
		case Rook:
			return BlackRook
		case Bishop:
			return BlackBishop
		case Knight:
			return BlackKnight
		case Pawn:
			return BlackPawn
		}
	}
	return NoPiece
}

func pieceTypeToInternal(pt PieceType) uint8 {
	switch pt {
	case Pawn:
		return piecePawn
	case Knight:
		return pieceKnight
	case Bishop:
		return pieceBishop
	case Rook:
		return pieceRook
	case Queen:
		return pieceQueen
	case King:
		return pieceKing
	default:
		return 0
	}
}

func internalToPieceType(pt uint8) PieceType {
	switch pt {
	case piecePawn:
		return Pawn
	case pieceKnight:
		return Knight
	case pieceBishop:
		return Bishop
	case pieceRook:
		return Rook
	case pieceQueen:
		return Queen
	case pieceKing:
		return King
	default:
		return NoPieceType
	}
}

func (m Move) S1() Square {
	return Square(m.From)
}

func (m Move) S2() Square {
	return Square(m.To)
}

func (m Move) Promo() PieceType {
	if m.promoteTo == 0 {
		return NoPieceType
	}
	return internalToPieceType(m.promoteTo)
}

func (m Move) HasTag(tag MoveTag) bool {
	switch tag {
	case Capture:
		return m.isCapture
	case Check:
		return m.isCheck
	case EnPassant:
		return m.isEnPassant
	case KingSideCastle:
		return m.isCastle && int(m.To) > int(m.From)
	case QueenSideCastle:
		return m.isCastle && int(m.To) < int(m.From)
	default:
		return false
	}
}

func (b *Board) Piece(sq Square) Piece {
	bit := Bitboard(uint64(1) << sq)
	if b.WhiteKing&bit != 0 {
		return WhiteKing
	}
	if b.WhiteQueens&bit != 0 {
		return WhiteQueen
	}
	if b.WhiteRooks&bit != 0 {
		return WhiteRook
	}
	if b.WhiteBishops&bit != 0 {
		return WhiteBishop
	}
	if b.WhiteKnights&bit != 0 {
		return WhiteKnight
	}
	if b.WhitePawns&bit != 0 {
		return WhitePawn
	}
	if b.BlackKing&bit != 0 {
		return BlackKing
	}
	if b.BlackQueens&bit != 0 {
		return BlackQueen
	}
	if b.BlackRooks&bit != 0 {
		return BlackRook
	}
	if b.BlackBishops&bit != 0 {
		return BlackBishop
	}
	if b.BlackKnights&bit != 0 {
		return BlackKnight
	}
	if b.BlackPawns&bit != 0 {
		return BlackPawn
	}
	return NoPiece
}

type Position struct {
	board Board
}

func (p *Position) Board() *Board {
	return &p.board
}

func (p *Position) Turn() Color {
	if p.board.turn {
		return White
	}
	return Black
}

func (p *Position) ValidMoves() []Move {
	return GenerateValidMoves(p.board)
}

func (p *Position) Update(move *Move) *Position {
	next := p.board
	if move == nil {
		next.turn = !next.turn
		next.enPassant = noEnPassant
		return &Position{board: next}
	}
	next = applyMove(next, *move)
	return &Position{board: next}
}

func castleString(mask uint8) CastleRights {
	var b strings.Builder
	if mask&castleWhiteKing != 0 {
		b.WriteByte('K')
	}
	if mask&castleWhiteQueen != 0 {
		b.WriteByte('Q')
	}
	if mask&castleBlackKing != 0 {
		b.WriteByte('k')
	}
	if mask&castleBlackQueen != 0 {
		b.WriteByte('q')
	}
	if b.Len() == 0 {
		return CastleRights("-")
	}
	return CastleRights(b.String())
}

func (p *Position) CastleRights() CastleRights {
	return castleString(p.board.castleRights)
}

func (p *Position) EnPassantSquare() Square {
	if p.board.enPassant >= 64 {
		return NoSquare
	}
	return Square(p.board.enPassant)
}

func (p *Position) Status() Method {
	moves := GenerateValidMoves(p.board)
	if len(moves) != 0 {
		return NoMethod
	}
	if isKingInCheckForSide(p.board, p.board.turn) {
		return Checkmate
	}
	return Stalemate
}

func mirrorFilesInRanks(bb uint64) uint64 {
	const k1 = 0x5555555555555555
	const k2 = 0x3333333333333333
	const k4 = 0x0f0f0f0f0f0f0f0f
	bb = ((bb >> 1) & k1) | ((bb & k1) << 1)
	bb = ((bb >> 2) & k2) | ((bb & k2) << 2)
	bb = ((bb >> 4) & k4) | ((bb & k4) << 4)
	return bb
}

func (p *Position) MarshalBinary() ([12]uint64, error) {
	// Legacy-compatible layout expected by the engine:
	// WK, WQ, WR, WB, WN, WP, BK, BQ, BR, BB, BN, BP.
	arr := [12]uint64{
		mirrorFilesInRanks(uint64(p.board.WhiteKing)),
		mirrorFilesInRanks(uint64(p.board.WhiteQueens)),
		mirrorFilesInRanks(uint64(p.board.WhiteRooks)),
		mirrorFilesInRanks(uint64(p.board.WhiteBishops)),
		mirrorFilesInRanks(uint64(p.board.WhiteKnights)),
		mirrorFilesInRanks(uint64(p.board.WhitePawns)),
		mirrorFilesInRanks(uint64(p.board.BlackKing)),
		mirrorFilesInRanks(uint64(p.board.BlackQueens)),
		mirrorFilesInRanks(uint64(p.board.BlackRooks)),
		mirrorFilesInRanks(uint64(p.board.BlackBishops)),
		mirrorFilesInRanks(uint64(p.board.BlackKnights)),
		mirrorFilesInRanks(uint64(p.board.BlackPawns)),
	}
	return arr, nil
}

type Game struct {
	position *Position
	moves    []*Move
}

type PushMoveOptions struct{}

type gameOption func(*Game) error

const startFEN = "rn1qkbnr/pppbpppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func defaultStartPosition() *Position {
	b := NewBoardFromFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	if b == nil {
		b = &Board{}
	}
	return &Position{board: *b}
}

func NewGame(options ...gameOption) *Game {
	g := &Game{position: defaultStartPosition()}
	for _, opt := range options {
		if opt != nil {
			_ = opt(g)
		}
	}
	return g
}

func (g *Game) Position() *Position {
	return g.position
}

func (g *Game) Move(move *Move, _ *PushMoveOptions) error {
	if move == nil {
		return errors.New("move is nil")
	}
	g.position = g.position.Update(move)
	cp := *move
	g.moves = append(g.moves, &cp)
	return nil
}

func (g *Game) Moves() []*Move {
	return g.moves
}

func FEN(fen string) (gameOption, error) {
	b := NewBoardFromFEN(fen)
	if b == nil {
		return nil, errors.New("invalid FEN")
	}
	return func(g *Game) error {
		g.position = &Position{board: *b}
		g.moves = nil
		return nil
	}, nil
}

type UCINotation struct{}
type AlgebraicNotation struct{}

type notationAPI struct{}

var Notation notationAPI

func parseSquare(s string) (Square, error) {
	if len(s) != 2 {
		return NoSquare, errors.New("invalid square")
	}
	file := s[0] - 'a'
	rank := s[1] - '1'
	if file > 7 || rank > 7 {
		return NoSquare, errors.New("invalid square")
	}
	return Square(int(rank)*8 + int(file)), nil
}

func (notationAPI) Decode(_ UCINotation, p *Position, uci string) (*Move, error) {
	if len(uci) < 4 {
		return nil, errors.New("invalid uci move")
	}
	from, err := parseSquare(uci[:2])
	if err != nil {
		return nil, err
	}
	to, err := parseSquare(uci[2:4])
	if err != nil {
		return nil, err
	}

	piece := p.board.Piece(from)
	if piece == NoPiece {
		return nil, fmt.Errorf("no piece on %s", uci[:2])
	}
	pieceType := pieceTypeToInternal(piece.Type())
	if pieceType == 0 {
		return nil, errors.New("invalid moving piece")
	}

	captured := p.board.Piece(to)
	isCapture := captured != NoPiece
	isEnPassant := false
	if piece.Type() == Pawn && p.board.enPassant < 64 && Square(p.board.enPassant) == to && captured == NoPiece {
		isCapture = true
		isEnPassant = true
	}
	isCastle := piece.Type() == King && (int(from)-int(to) == 2 || int(to)-int(from) == 2)

	move := &Move{
		From:        uint8(from),
		To:          uint8(to),
		PieceType:   pieceType,
		isCapture:   isCapture,
		isEnPassant: isEnPassant,
		isCastle:    isCastle,
	}

	if len(uci) == 5 {
		switch uci[4] {
		case 'q', 'Q':
			move.promoteTo = pieceQueen
		case 'r', 'R':
			move.promoteTo = pieceRook
		case 'b', 'B':
			move.promoteTo = pieceBishop
		case 'n', 'N':
			move.promoteTo = pieceKnight
		default:
			return nil, errors.New("invalid promotion suffix")
		}
	}

	next := applyMove(p.board, *move)
	move.isCheck = isKingInCheckForSide(next, !p.board.turn)

	return move, nil
}

func (AlgebraicNotation) Decode(p *Position, san string) (*Move, error) {
	trimmed := strings.TrimSpace(strings.TrimRight(strings.TrimRight(san, "+"), "#"))
	for _, mv := range p.ValidMoves() {
		uci := mv.String()
		if trimmed == uci {
			cp := mv
			return &cp, nil
		}
	}
	return nil, errors.New("SAN decode not available in compatibility mode")
}
