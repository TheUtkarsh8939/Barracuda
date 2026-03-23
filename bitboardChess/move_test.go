package bitboardchess

import (
	"testing"
)

// Test to verify squareToAlgebraic function
func TestSquareToAlgebraic(t *testing.T) {
	tests := []struct {
		square   uint8
		expected string
	}{
		{0, "a1"},
		{7, "h1"},
		{8, "a2"},
		{63, "h8"},
	}

	for _, test := range tests {
		result := squareToAlgebraic(test.square)
		if result != test.expected {
			t.Errorf("squareToAlgebraic(%d) = %s; expected %s", test.square, result, test.expected)
		}
	}
}
