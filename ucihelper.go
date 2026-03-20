package main

import (
	"strconv"
	"strings"
)

// parseGoCmd parses a UCI "go" command string into a SearchOptions struct.
//
// The UCI "go" command can contain various options, for example:
//
//	go depth 6
//	go wtime 60000 btime 60000 winc 1000 binc 1000
//	go infinite
//
// Unrecognized tokens are silently ignored, and each option reads the
// next token as its value (standard UCI key-value format).
// Current search launcher primarily uses depth/infinite; clock fields are parsed for future time management.
func parseGoCmd(cmd string) *SearchOptions {
	result := &SearchOptions{}
	tokens := strings.Split(cmd, " ")
	for i, token := range tokens {
		option := token
		// Safely peek at the next token as the value for this option.
		value := ""
		if (i + 1) < len(tokens) {
			value = tokens[i+1]
		}
		if option == "depth" {
			res, _ := strconv.Atoi(value)
			result.depth = uint8(res)
		} else if option == "infinite" {
			// "go infinite" means search until a "stop" command is received.
			result.isInf = true
			result.depth = 255 // Effectively unlimited depth
		} else if option == "wtime" {
			result.whiteTime, _ = strconv.Atoi(value)
		} else if option == "btime" {
			result.blackTime, _ = strconv.Atoi(value)
		} else if option == "binc" {
			result.binc, _ = strconv.Atoi(value)
		} else if option == "winc" {
			result.winc, _ = strconv.Atoi(value)
		}
	}
	return result
}
