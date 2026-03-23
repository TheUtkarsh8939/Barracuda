#!/usr/bin/env python3
"""Generate Go code for magic-bitboard masks, magics, and attack tables.

This script supports two modes:
1) Deterministic/fast table generation using occupancy-index tables (always works)
2) Brute-force magic search (slow) to emit real magic multipliers

Usage examples:
    python generate_magic_bitboards.py --mode occupancy > generated_magic_bitboards.go
    python generate_magic_bitboards.py --mode brute > generated_magic_bitboards.go
"""

from __future__ import annotations

import argparse
import random
from dataclasses import dataclass


def bb(sq: int) -> int:
    return 1 << sq


def file_of(sq: int) -> int:
    return sq & 7


def rank_of(sq: int) -> int:
    return sq >> 3


def on_board(file: int, rank: int) -> bool:
    return 0 <= file < 8 and 0 <= rank < 8


def rook_mask(square: int) -> int:
    f = file_of(square)
    r = rank_of(square)
    out = 0
    for rr in range(r + 1, 7):
        out |= bb(rr * 8 + f)
    for rr in range(r - 1, 0, -1):
        out |= bb(rr * 8 + f)
    for ff in range(f + 1, 7):
        out |= bb(r * 8 + ff)
    for ff in range(f - 1, 0, -1):
        out |= bb(r * 8 + ff)
    return out


def bishop_mask(square: int) -> int:
    f = file_of(square)
    r = rank_of(square)
    out = 0

    ff, rr = f + 1, r + 1
    while ff <= 6 and rr <= 6:
        out |= bb(rr * 8 + ff)
        ff += 1
        rr += 1

    ff, rr = f - 1, r + 1
    while ff >= 1 and rr <= 6:
        out |= bb(rr * 8 + ff)
        ff -= 1
        rr += 1

    ff, rr = f + 1, r - 1
    while ff <= 6 and rr >= 1:
        out |= bb(rr * 8 + ff)
        ff += 1
        rr -= 1

    ff, rr = f - 1, r - 1
    while ff >= 1 and rr >= 1:
        out |= bb(rr * 8 + ff)
        ff -= 1
        rr -= 1

    return out


def rook_attacks_on_the_fly(square: int, blockers: int) -> int:
    f = file_of(square)
    r = rank_of(square)
    out = 0

    for rr in range(r + 1, 8):
        sq = rr * 8 + f
        out |= bb(sq)
        if blockers & bb(sq):
            break

    for rr in range(r - 1, -1, -1):
        sq = rr * 8 + f
        out |= bb(sq)
        if blockers & bb(sq):
            break

    for ff in range(f + 1, 8):
        sq = r * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break

    for ff in range(f - 1, -1, -1):
        sq = r * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break

    return out


def bishop_attacks_on_the_fly(square: int, blockers: int) -> int:
    f = file_of(square)
    r = rank_of(square)
    out = 0

    ff, rr = f + 1, r + 1
    while ff <= 7 and rr <= 7:
        sq = rr * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break
        ff += 1
        rr += 1

    ff, rr = f - 1, r + 1
    while ff >= 0 and rr <= 7:
        sq = rr * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break
        ff -= 1
        rr += 1

    ff, rr = f + 1, r - 1
    while ff <= 7 and rr >= 0:
        sq = rr * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break
        ff += 1
        rr -= 1

    ff, rr = f - 1, r - 1
    while ff >= 0 and rr >= 0:
        sq = rr * 8 + ff
        out |= bb(sq)
        if blockers & bb(sq):
            break
        ff -= 1
        rr -= 1

    return out


def bitscan(mask: int) -> list[int]:
    out: list[int] = []
    while mask:
        lsb = (mask & -mask).bit_length() - 1
        out.append(lsb)
        mask &= mask - 1
    return out


def index_to_occ(index: int, mask: int) -> int:
    out = 0
    bits_ = bitscan(mask)
    for i, sq in enumerate(bits_):
        if index & (1 << i):
            out |= bb(sq)
    return out


def random_u64() -> int:
    return random.getrandbits(64)


def random_magic_candidate() -> int:
    return random_u64() & random_u64() & random_u64()


@dataclass
class MagicEntry:
    mask: int
    magic: int
    shift: int
    attacks: list[int]


def find_magic(square: int, bishop: bool, max_tries: int = 2_000_000) -> MagicEntry:
    mask = bishop_mask(square) if bishop else rook_mask(square)
    bits_ = mask.bit_count()
    shift = 64 - bits_
    size = 1 << bits_

    occs = [index_to_occ(i, mask) for i in range(size)]
    attacks = [
        bishop_attacks_on_the_fly(square, occ) if bishop else rook_attacks_on_the_fly(square, occ)
        for occ in occs
    ]

    for _ in range(max_tries):
        magic = random_magic_candidate()
        if ((mask * magic) & 0xFF00_0000_0000_0000).bit_count() < 6:
            continue
        used = [None] * size
        fail = False
        for occ, atk in zip(occs, attacks):
            idx = ((occ * magic) & 0xFFFF_FFFF_FFFF_FFFF) >> shift
            cur = used[idx]
            if cur is None:
                used[idx] = atk
            elif cur != atk:
                fail = True
                break
        if not fail:
            out_attacks = [0] * size
            for occ, atk in zip(occs, attacks):
                idx = ((occ * magic) & 0xFFFF_FFFF_FFFF_FFFF) >> shift
                out_attacks[idx] = atk
            return MagicEntry(mask=mask, magic=magic, shift=shift, attacks=out_attacks)

    raise RuntimeError(f"failed to find magic for square {square}, bishop={bishop}")


def emit_array_u64(name: str, arr: list[int]) -> None:
    print(f"var {name} = [64]uint64{{")
    for x in arr:
        print(f"\t0x{x:016x},")
    print("}")
    print()


def emit_array_bb(name: str, arr: list[int]) -> None:
    print(f"var {name} = [64]Bitboard{{")
    for x in arr:
        print(f"\t0x{x:016x},")
    print("}")
    print()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=["occupancy", "brute"], default="occupancy")
    parser.add_argument("--seed", type=int, default=1337)
    args = parser.parse_args()

    random.seed(args.seed)

    rook_masks = [rook_mask(sq) for sq in range(64)]
    bishop_masks = [bishop_mask(sq) for sq in range(64)]

    rook_magics = [0] * 64
    bishop_magics = [0] * 64

    rook_relevant_bits = [m.bit_count() for m in rook_masks]
    bishop_relevant_bits = [m.bit_count() for m in bishop_masks]

    rook_offsets = [0] * 64
    bishop_offsets = [0] * 64

    rook_total = 0
    bishop_total = 0
    for sq in range(64):
        rook_offsets[sq] = rook_total
        bishop_offsets[sq] = bishop_total
        rook_total += 1 << rook_relevant_bits[sq]
        bishop_total += 1 << bishop_relevant_bits[sq]

    rook_attack_table = [0] * rook_total
    bishop_attack_table = [0] * bishop_total

    if args.mode == "brute":
        rook_entries = [find_magic(sq, bishop=False) for sq in range(64)]
        bishop_entries = [find_magic(sq, bishop=True) for sq in range(64)]

        for sq, e in enumerate(rook_entries):
            rook_magics[sq] = e.magic
            for i, atk in enumerate(e.attacks):
                rook_attack_table[rook_offsets[sq] + i] = atk

        for sq, e in enumerate(bishop_entries):
            bishop_magics[sq] = e.magic
            for i, atk in enumerate(e.attacks):
                bishop_attack_table[bishop_offsets[sq] + i] = atk
    else:
        # Occupancy-index fallback tables (deterministic and always valid).
        for sq in range(64):
            subset_count = 1 << rook_relevant_bits[sq]
            for idx in range(subset_count):
                occ = index_to_occ(idx, rook_masks[sq])
                rook_attack_table[rook_offsets[sq] + idx] = rook_attacks_on_the_fly(sq, occ)
        for sq in range(64):
            subset_count = 1 << bishop_relevant_bits[sq]
            for idx in range(subset_count):
                occ = index_to_occ(idx, bishop_masks[sq])
                bishop_attack_table[bishop_offsets[sq] + idx] = bishop_attacks_on_the_fly(sq, occ)

    print("// Code generated by generate_magic_bitboards.py; DO NOT EDIT.")
    print("package bitboardchess")
    print()

    emit_array_bb("GeneratedRookMasks", rook_masks)
    emit_array_bb("GeneratedBishopMasks", bishop_masks)
    emit_array_u64("GeneratedRookMagics", rook_magics)
    emit_array_u64("GeneratedBishopMagics", bishop_magics)

    print("var GeneratedRookOffsets = [64]int{")
    for x in rook_offsets:
        print(f"\t{x},")
    print("}")
    print()

    print("var GeneratedBishopOffsets = [64]int{")
    for x in bishop_offsets:
        print(f"\t{x},")
    print("}")
    print()

    print("var GeneratedRookAttackTable = []Bitboard{")
    for x in rook_attack_table:
        print(f"\t0x{x:016x},")
    print("}")
    print()

    print("var GeneratedBishopAttackTable = []Bitboard{")
    for x in bishop_attack_table:
        print(f"\t0x{x:016x},")
    print("}")


if __name__ == "__main__":
    main()
