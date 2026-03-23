package main

import (
	"fmt"
	"strconv"
	"strings"
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
