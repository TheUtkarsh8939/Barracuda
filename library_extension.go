package main

import (
	"fmt"
	"strconv"
	"strings"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

// parseUCISquare converts coordinates like "e2" into 0..63 square index.
func parseUCISquare(coord string) (uint8, error) {
	if len(coord) != 2 {
		return 0, fmt.Errorf("invalid coordinate %q", coord)
	}
	file := coord[0] - 'a'
	rank := coord[1] - '1'
	if file > 7 || rank > 7 {
		return 0, fmt.Errorf("invalid coordinate %q", coord)
	}
	return uint8(int(rank)*8 + int(file)), nil
}

// formatUCISquare converts a square index (0..63) to UCI coordinate notation.
func formatUCISquare(sq uint8) string {
	return string([]byte{'a' + (sq % 8), '1' + (sq / 8)})
}

// clampUint8String parses an integer token and clamps it into uint8 bounds.
func clampUint8String(token string, fallback uint8) uint8 {
	v, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil {
		return fallback
	}
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func sameMove(a *chess.Move, b *chess.Move) bool {
	if a == nil || b == nil {
		return false
	}
	return a.S1() == b.S1() && a.S2() == b.S2() && a.Promo() == b.Promo()
}

func pieceSANLetter(pt chess.PieceType) string {
	switch pt {
	case chess.King:
		return "K"
	case chess.Queen:
		return "Q"
	case chess.Rook:
		return "R"
	case chess.Bishop:
		return "B"
	case chess.Knight:
		return "N"
	default:
		return ""
	}
}

func promotionSuffix(pt chess.PieceType) string {
	switch pt {
	case chess.Queen:
		return "=Q"
	case chess.Rook:
		return "=R"
	case chess.Bishop:
		return "=B"
	case chess.Knight:
		return "=N"
	default:
		return ""
	}
}

func fileChar(sq chess.Square) byte {
	return byte('a' + (uint8(sq) % 8))
}

func rankChar(sq chess.Square) byte {
	return byte('1' + (uint8(sq) / 8))
}

func disambiguation(pos *chess.Position, m *chess.Move, movingPiece chess.Piece) string {
	if movingPiece.Type() == chess.Pawn {
		return ""
	}

	others := make([]chess.Move, 0)
	for _, cand := range pos.ValidMoves() {
		cp := cand
		if sameMove(&cp, m) {
			continue
		}
		if cp.S2() != m.S2() {
			continue
		}
		p := pos.Board().Piece(cp.S1())
		if p.Type() == movingPiece.Type() && p.Color() == movingPiece.Color() {
			others = append(others, cp)
		}
	}

	if len(others) == 0 {
		return ""
	}

	from := m.S1()
	anySameFile := false
	anySameRank := false
	for _, other := range others {
		if fileChar(other.S1()) == fileChar(from) {
			anySameFile = true
		}
		if rankChar(other.S1()) == rankChar(from) {
			anySameRank = true
		}
	}

	if !anySameFile {
		return string([]byte{fileChar(from)})
	}
	if !anySameRank {
		return string([]byte{rankChar(from)})
	}
	return string([]byte{fileChar(from), rankChar(from)})
}

func toSAN(pos *chess.Position, m *chess.Move) (string, error) {
	if m == nil {
		return "", fmt.Errorf("move is nil")
	}

	movingPiece := pos.Board().Piece(m.S1())
	if movingPiece == chess.NoPiece {
		return "", fmt.Errorf("no piece on source square")
	}

	if m.HasTag(chess.KingSideCastle) {
		suffix := ""
		next := pos.Update(m)
		if next.Status() == chess.Checkmate {
			suffix = "#"
		} else if m.HasTag(chess.Check) {
			suffix = "+"
		}
		return "O-O" + suffix, nil
	}
	if m.HasTag(chess.QueenSideCastle) {
		suffix := ""
		next := pos.Update(m)
		if next.Status() == chess.Checkmate {
			suffix = "#"
		} else if m.HasTag(chess.Check) {
			suffix = "+"
		}
		return "O-O-O" + suffix, nil
	}

	isCapture := m.HasTag(chess.Capture) || m.HasTag(chess.EnPassant)
	dest := formatUCISquare(uint8(m.S2()))

	var b strings.Builder
	if movingPiece.Type() == chess.Pawn {
		if isCapture {
			b.WriteByte(fileChar(m.S1()))
			b.WriteByte('x')
		}
		b.WriteString(dest)
	} else {
		b.WriteString(pieceSANLetter(movingPiece.Type()))
		b.WriteString(disambiguation(pos, m, movingPiece))
		if isCapture {
			b.WriteByte('x')
		}
		b.WriteString(dest)
	}

	if promo := promotionSuffix(m.Promo()); promo != "" {
		b.WriteString(promo)
	}

	next := pos.Update(m)
	if next.Status() == chess.Checkmate {
		b.WriteByte('#')
	} else if m.HasTag(chess.Check) {
		b.WriteByte('+')
	}

	return b.String(), nil
}

// movesToAlgebraicNotation converts a sequence of bitboard moves into SAN strings.
func movesToAlgebraicNotation(moves []*chess.Move) ([]string, error) {
	game := chess.NewGame()
	sanMoves := make([]string, 0, len(moves))

	for i, move := range moves {
		if move == nil {
			return nil, fmt.Errorf("nil move at index %d", i)
		}

		pos := game.Position()
		var legal *chess.Move
		for _, cand := range pos.ValidMoves() {
			cp := cand
			if sameMove(&cp, move) {
				legal = &cp
				break
			}
		}
		if legal == nil {
			return nil, fmt.Errorf("illegal move at index %d: %s", i, move.String())
		}

		san, err := toSAN(pos, legal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert move at index %d: %w", i, err)
		}
		sanMoves = append(sanMoves, san)

		if err := game.Move(legal, &chess.PushMoveOptions{}); err != nil {
			return nil, fmt.Errorf("failed to apply move at index %d: %w", i, err)
		}
	}

	return sanMoves, nil
}

func normalizeSANForComparison(token string) string {
	s := strings.TrimSpace(token)
	s = strings.ReplaceAll(s, "0-0-0", "O-O-O")
	s = strings.ReplaceAll(s, "0-0", "O-O")
	s = strings.TrimSuffix(s, " e.p.")
	s = strings.TrimSuffix(s, "e.p.")

	for len(s) > 0 {
		last := s[len(s)-1]
		if last == '+' || last == '#' || last == '!' || last == '?' {
			s = s[:len(s)-1]
			continue
		}
		break
	}

	return s
}

func sanEquivalent(a string, b string) bool {
	return normalizeSANForComparison(a) == normalizeSANForComparison(b)
}

// moveFromAlgebraicToUCI converts one SAN token to its UCI form for the given position.
func moveFromAlgebraicToUCI(pos *chess.Position, san string) (string, error) {
	if pos == nil {
		return "", fmt.Errorf("position is nil")
	}

	sanToken := strings.TrimSpace(san)
	if sanToken == "" {
		return "", fmt.Errorf("empty SAN token")
	}

	matchCount := 0
	selectedUCI := ""

	for _, cand := range pos.ValidMoves() {
		cp := cand
		candidateSAN, err := toSAN(pos, &cp)
		if err != nil {
			continue
		}
		if sanEquivalent(candidateSAN, sanToken) {
			matchCount++
			selectedUCI = cp.String()
		}
	}

	if matchCount == 0 {
		return "", fmt.Errorf("no legal move matches SAN %q", sanToken)
	}
	if matchCount > 1 {
		return "", fmt.Errorf("ambiguous SAN %q", sanToken)
	}

	return selectedUCI, nil
}
