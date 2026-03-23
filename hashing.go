// hashing.go provides fast Zobrist-style hashing for TT/PV lookup and move-to-move updates.
//
// The library's built-in position.Hash() uses MarshalBinary + md5.Sum (~690 ns per call),
// which is too slow for the search hot path. Two strategies replace it:
//
//   - fastPosHash:  full board scan, O(64 squares). Used once per search root.
//   - fastChildHash: incremental XOR delta, O(move deltas). Used inside every recursive
//     minimax call, avoiding a full rescan at each node.
//
// Keys are Zobrist-style: each (piece, square) pair and each game-state feature (castle
// rights, en passant file, side to move) gets a unique pseudo-random 64-bit value.
// XOR of all active keys is the position hash. Any change to the position flips only the
// affected XOR terms — the rest of the hash is unchanged (incremental property).
//
// Keys are indexed directly by chess.Piece (int8: 0=NoPiece, 1=WhiteKing..12=BlackPawn)
// via fastHashPieceByID, avoiding Color() and Type() method calls in the hot path.
package main

import chess "github.com/TheUtkarsh8939/bitboardChess"

// fastHashPieceKeys[color][pieceType][square] — intermediate key layout used during init.
// Not used in the hot path; exists so fastHashPieceByID can be derived from it.
// Array dimensions: [Color (0-2: NoColor, White, Black)][PieceType (0-6: NoPieceType..Pawn)][Square (0-63)].
// NoColor (0) and NoPieceType (0) entries are unused but present to keep indexing consistent.
var fastHashPieceKeys [3][7][64]uint64

// fastHashPieceByID[piece][square] — the hot-path lookup table.
// Indexed by chess.Piece (int8: 0=NoPiece, 1=WhiteKing..12=BlackPawn), directly usable
// without calling piece.Color() or piece.Type(). Values are derived from fastHashPieceKeys.
var fastHashPieceByID [13][64]uint64

// fastHashCastleKeys[mask] — 16 keys for every combination of the 4 castle-rights bits.
// The mask is computed by fastCastleMask; the key is XOR'd into the hash for this position's rights.
var fastHashCastleKeys [16]uint64

// fastHashEnPassantFileKeys[file] — one key per file (0=a..7=h).
// Only the file matters; en passant captures always happen from the 5th rank, so the rank is implicit.
var fastHashEnPassantFileKeys [8]uint64

// fastHashTurnKey is XOR'd into the hash whenever it is Black's turn to move.
var fastHashTurnKey uint64

// fastCastleMask encodes all currently-active castle rights into a 4-bit integer:
//
//	bit 0 set → White kingside  (K)
//	bit 1 set → White queenside (Q)
//	bit 2 set → Black kingside  (k)
//	bit 3 set → Black queenside (q)
//
// The result (0–15) indexes directly into fastHashCastleKeys. A single string scan is
// faster than calling CanCastle() four separate times.
func fastCastleMask(castleRights chess.CastleRights) int {
	mask := 0
	s := string(castleRights)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'K':
			mask |= 1
		case 'Q':
			mask |= 2
		case 'k':
			mask |= 4
		case 'q':
			mask |= 8
		}
	}
	return mask
}

// fastEnPassantKey returns the Zobrist key for the current en passant state.
// Only the file of the ep square is hashed (not the full square) because en passant captures
// can only happen from the 5th rank — the rank is always implicit from the context.
// Returns 0 if there is no en passant square; XOR with 0 is a no-op so no branch is needed at call sites.
func fastEnPassantKey(position *chess.Position) uint64 {
	ep := position.EnPassantSquare()
	if ep == chess.NoSquare {
		return 0
	}
	return fastHashEnPassantFileKeys[int(ep)%8]
}

// splitmix64 is a fast, high-quality 64-bit bijective hash mixer used as a PRNG.
// It is called only during initFastHashKeys to generate pseudo-random Zobrist keys.
// Each invocation advances the seed and returns a well-distributed 64-bit output with
// zero correlation between consecutive outputs. This ensures XOR collisions among keys
// are negligibly rare, which is the core requirement for low-collision Zobrist hashing.
// Reference algorithm: https://prng.di.unimi.it/splitmix64.c
func splitmix64(seed uint64) uint64 {
	seed += 0x9e3779b97f4a7c15
	z := seed
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// initFastHashKeys generates all Zobrist-style random keys used by fastPosHash and fastChildHash.
// Called once at program startup via the init() function below.
//
// Generated tables:
//
//	fastHashPieceKeys[color][pieceType][sq]  — primary key grid, 3×7×64 = 1344 entries
//	fastHashPieceByID[piece][sq]             — same keys re-indexed by raw chess.Piece int8
//	fastHashCastleKeys[mask]                 — 16 keys, one per 4-bit castle-rights combination
//	fastHashEnPassantFileKeys[file]          — 8 keys, one per ep file
//	fastHashTurnKey                          — single key XOR'd in when Black is to move
func initFastHashKeys() {
	seed := uint64(0x7f4a7c159e3779b9)
	for color := 0; color < 3; color++ {
		for pt := 0; pt < 7; pt++ {
			for sq := 0; sq < 64; sq++ {
				seed = splitmix64(seed)
				fastHashPieceKeys[color][pt][sq] = seed
			}
		}
	}

	// Re-index into fastHashPieceByID so the hot path can look up keys by raw chess.Piece
	// value (an int8 iota) without calling the Color() or Type() accessor methods.
	for sq := 0; sq < 64; sq++ {
		fastHashPieceByID[chess.WhiteKing][sq] = fastHashPieceKeys[chess.White][chess.King][sq]
		fastHashPieceByID[chess.WhiteQueen][sq] = fastHashPieceKeys[chess.White][chess.Queen][sq]
		fastHashPieceByID[chess.WhiteRook][sq] = fastHashPieceKeys[chess.White][chess.Rook][sq]
		fastHashPieceByID[chess.WhiteBishop][sq] = fastHashPieceKeys[chess.White][chess.Bishop][sq]
		fastHashPieceByID[chess.WhiteKnight][sq] = fastHashPieceKeys[chess.White][chess.Knight][sq]
		fastHashPieceByID[chess.WhitePawn][sq] = fastHashPieceKeys[chess.White][chess.Pawn][sq]
		fastHashPieceByID[chess.BlackKing][sq] = fastHashPieceKeys[chess.Black][chess.King][sq]
		fastHashPieceByID[chess.BlackQueen][sq] = fastHashPieceKeys[chess.Black][chess.Queen][sq]
		fastHashPieceByID[chess.BlackRook][sq] = fastHashPieceKeys[chess.Black][chess.Rook][sq]
		fastHashPieceByID[chess.BlackBishop][sq] = fastHashPieceKeys[chess.Black][chess.Bishop][sq]
		fastHashPieceByID[chess.BlackKnight][sq] = fastHashPieceKeys[chess.Black][chess.Knight][sq]
		fastHashPieceByID[chess.BlackPawn][sq] = fastHashPieceKeys[chess.Black][chess.Pawn][sq]
	}
	for i := 0; i < 16; i++ {
		seed = splitmix64(seed)
		fastHashCastleKeys[i] = seed
	}
	for i := 0; i < 8; i++ {
		seed = splitmix64(seed)
		fastHashEnPassantFileKeys[i] = seed
	}
	seed = splitmix64(seed)
	fastHashTurnKey = seed
}

// init ensures all Zobrist keys are precomputed before any search begins.
// Using Go's init() mechanism guarantees this runs exactly once before main().
func init() {
	initFastHashKeys()
}

// fastPosHash computes a 64-bit Zobrist-style hash for a complete position from scratch.
//
// Algorithm: XOR together the random key for each (piece, square) pair on the board, then
// XOR in the castle-rights key and (if applicable) the en passant file key and turn key:
//
//	hash = XOR of fastHashPieceByID[piece][sq] for every occupied square
//	     ^ fastHashCastleKeys[castleMask]
//	     ^ fastHashEnPassantFileKeys[epFile]  (skipped if no ep square)
//	     ^ fastHashTurnKey                    (only if Black to move)
//
// This is O(64) and is called only once at the root of each iterative-deepening iteration.
// Inside the recursive search, fastChildHash performs an O(K) incremental update instead.
// The hash is non-cryptographic — its purpose is fast TT/PV indexing with a low collision rate.
func fastPosHash(position *chess.Position) uint64 {
	board := position.Board()
	h := uint64(0)
	for sq := 0; sq < 64; sq++ {
		p := board.Piece(chess.Square(sq))
		if p == chess.NoPiece {
			continue
		}
		h ^= fastHashPieceByID[p][sq]
	}

	h ^= fastHashCastleKeys[fastCastleMask(position.CastleRights())]
	h ^= fastEnPassantKey(position)

	if position.Turn() == chess.Black {
		h ^= fastHashTurnKey
	}

	return h
}

// fastChildHash derives the child position's hash from the parent hash by XOR-ing only
// the keys that change as a result of the move, without rescanning all 64 squares.
//
// XOR deltas applied in order:
//  1. Turn key — side to move always flips after a move.
//  2. Castle rights — XOR out parent mask, XOR in child mask (may shrink after king/rook move).
//  3. En passant file — XOR out parent ep key, XOR in child ep key (ep square resets each ply).
//  4. Moving piece at source — remove piece from its origin square.
//  5. Captured piece — remove from destination square (or offset square for en passant captures).
//  6. Placed piece at destination — the moved piece (or promoted piece) lands here.
//  7. Castling rook — if the king is castling, the rook also moves; XOR out old and in new square.
//
// Falls back to a full fastPosHash scan if the source square contains no piece, which
// should not happen in legal play but is included as a safety net.
// This is the critical hot-path function: called once per move at every minimax recursion node.
func fastChildHash(parent *chess.Position, child *chess.Position, move *chess.Move, parentHash uint64) uint64 {
	h := parentHash
	parentBoard := parent.Board()
	from := int(move.S1())
	to := int(move.S2())
	moved := parentBoard.Piece(move.S1())

	if moved == chess.NoPiece {
		// Safety fallback — should not occur in legal play.
		return fastPosHash(child)
	}

	// 1. Side to move toggles every ply.
	h ^= fastHashTurnKey

	// 2. Castle rights may be lost when a king or rook moves; XOR out the old mask and in the new.
	h ^= fastHashCastleKeys[fastCastleMask(parent.CastleRights())]
	h ^= fastHashCastleKeys[fastCastleMask(child.CastleRights())]

	// 3. En passant file resets after each ply; XOR out the old ep key and in the new.
	h ^= fastEnPassantKey(parent)
	h ^= fastEnPassantKey(child)

	// 4. Remove the moving piece from its source square.
	h ^= fastHashPieceByID[moved][from]

	// 5. Remove any captured piece. For en passant, the captured pawn sits one rank behind
	//    the destination square (not on the destination itself).
	if move.HasTag(chess.Capture) {
		captureSquare := move.S2()
		if move.HasTag(chess.EnPassant) {
			// En passant: White captures upward (S2+8 is the white pawn's rank above captured pawn),
			// so the captured Black pawn is at S2-8; and vice versa for Black.
			if moved.Color() == chess.White {
				captureSquare = move.S2() - 8
			} else {
				captureSquare = move.S2() + 8
			}
		}
		captured := parentBoard.Piece(captureSquare)
		if captured != chess.NoPiece {
			h ^= fastHashPieceByID[captured][int(captureSquare)]
		}
	}

	// 6. Place the piece (or the promoted piece) on the destination square.
	placed := moved
	if promo := move.Promo(); promo != chess.NoPieceType {
		// On promotion the pawn is replaced by the new piece type.
		placed = chess.NewPiece(promo, moved.Color())
	}
	h ^= fastHashPieceByID[placed][to]

	// 7. Castling also moves the rook — XOR out old rook square, XOR in new rook square.
	//    King movement is already handled by steps 4 and 6 above.
	if move.HasTag(chess.KingSideCastle) {
		if moved.Color() == chess.White {
			h ^= fastHashPieceByID[chess.WhiteRook][int(chess.H1)] // rook leaves H1
			h ^= fastHashPieceByID[chess.WhiteRook][int(chess.F1)] // rook arrives F1
		} else {
			h ^= fastHashPieceByID[chess.BlackRook][int(chess.H8)]
			h ^= fastHashPieceByID[chess.BlackRook][int(chess.F8)]
		}
	} else if move.HasTag(chess.QueenSideCastle) {
		if moved.Color() == chess.White {
			h ^= fastHashPieceByID[chess.WhiteRook][int(chess.A1)] // rook leaves A1
			h ^= fastHashPieceByID[chess.WhiteRook][int(chess.D1)] // rook arrives D1
		} else {
			h ^= fastHashPieceByID[chess.BlackRook][int(chess.A8)]
			h ^= fastHashPieceByID[chess.BlackRook][int(chess.D8)]
		}
	}

	return h
}

// fastNullHash derives the child hash for a null move ("pass") from the parent hash.
// In a null move no piece squares change; only side-to-move and state fields can change.
func fastNullHash(parent *chess.Position, child *chess.Position, parentHash uint64) uint64 {
	h := parentHash

	// Side to move always flips on a null move.
	h ^= fastHashTurnKey

	// Castle rights usually stay unchanged, but keep this symmetric with normal hashing.
	h ^= fastHashCastleKeys[fastCastleMask(parent.CastleRights())]
	h ^= fastHashCastleKeys[fastCastleMask(child.CastleRights())]

	// En passant is cleared or updated by the child position state.
	h ^= fastEnPassantKey(parent)
	h ^= fastEnPassantKey(child)

	return h
}
