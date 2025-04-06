package main

import (
	"strconv"
	"strings"
)

func parseGoCmd(cmd string) *SearchOptions {
	result := &SearchOptions{}
	tokens := strings.Split(cmd, " ")
	for i, token := range tokens {
		option := token
		value := ""
		if (i + 1) < len(tokens) {
			value = tokens[i+1]
		}
		if option == "depth" {
			res, _ := strconv.Atoi(value)
			result.depth = uint8(res)
		} else if option == "infinite" {
			result.isInf = true
			result.depth = 255
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
