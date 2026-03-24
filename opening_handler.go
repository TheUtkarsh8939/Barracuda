package main

import (
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

type openingBook []string

func initBook() *openingBook {
	book := &openingBook{}

	data, err := os.ReadFile("./openings.txt")
	if err != nil {
		return book
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		*book = append(*book, strings.TrimRight(line, " \t\r"))
	}

	return book
}

func randomOpeningMove(lines []string, rng *rand.Rand) string {
	firstMoves := make([]string, 0)
	for _, line := range lines {
		moves := strings.Fields(line)
		if len(moves) == 0 {
			continue
		}
		firstMoves = append(firstMoves, moves[0])
	}

	if len(firstMoves) == 0 {
		return ""
	}

	return firstMoves[rng.Intn(len(firstMoves))]
}

func findNextMove(algebraicMoves []string, book *openingBook) string {
	if book == nil || len(*book) == 0 {
		return ""
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	lines := *book

	queryMoves := algebraicMoves
	if len(queryMoves) == 0 {
		return randomOpeningMove(lines, rng)
	}

	prefix := strings.Join(queryMoves, " ")
	prefixWithSpace := prefix + " "

	// In the sorted book, all strings with the same prefix are contiguous.
	start := sort.Search(len(lines), func(i int) bool {
		return lines[i] >= prefix
	})
	end := sort.Search(len(lines), func(i int) bool {
		return lines[i] > prefix+"\xff"
	})

	candidateNextMoves := make([]string, 0)
	for i := start; i < end; i++ {
		line := lines[i]
		if line != prefix && !strings.HasPrefix(line, prefixWithSpace) {
			continue
		}

		lineMoves := strings.Fields(line)
		if len(lineMoves) <= len(queryMoves) {
			continue
		}
		candidateNextMoves = append(candidateNextMoves, lineMoves[len(queryMoves)])
	}

	if len(candidateNextMoves) == 0 {
		return randomOpeningMove(lines, rng)
	}

	// Randomly select one matching opening and return its next move.
	return candidateNextMoves[rng.Intn(len(candidateNextMoves))]
}
