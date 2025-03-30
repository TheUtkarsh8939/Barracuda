package main

//Stores the move to killer move table
func storeKillerMove(depth uint8, move Move) {
	if len(killerMoveTable[depth]) < 2 {
		killerMoveTable[depth] = append(killerMoveTable[depth], move)
	} else {
		killerMoveTable[depth][1] = killerMoveTable[depth][0] // Shift old move
		killerMoveTable[depth][0] = move                      // Store new killer move
	}
}

//Sets the killer Move table to support the next generation of moves
func resetKillerMoveTable(table map[uint8][]Move) map[uint8][]Move {
	if len(table) < 3 {
		return table
	}
	ntTable := make(map[uint8][]Move)
	for i := uint8(2); i < uint8(len(table)); i++ {
		ntTable[i] = table[i-2]
	}
	return ntTable
}
