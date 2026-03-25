package main

import (
	"fmt"
	"runtime"
	"time"

	chess "github.com/TheUtkarsh8939/bitboardChess"
)

// Sink variables make benchmark results observable so the compiler cannot optimize calls away.
var (
	benchIntSink    int
	benchUint64Sink uint64
	benchBoolSink   bool
	benchPosSink    *chess.Position
	benchErrSink    error
)

// cpuPercent converts CPU-time delta over wall-time delta into a 0..100 percentage.
func cpuPercent(cpuDeltaSeconds float64, elapsed time.Duration) float64 {
	elapsedSeconds := elapsed.Seconds()
	if elapsedSeconds <= 0 {
		return 0
	}
	procs := float64(runtime.GOMAXPROCS(0))
	if procs <= 0 {
		procs = 1
	}
	percent := (cpuDeltaSeconds / elapsedSeconds) * 100.0 / procs
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

// runBenchmark executes fn exactly calls times and prints elapsed and CPU usage.
func runBenchmark(functionName string, calls int, fn func(iter int)) {
	startCPU := readCPUSeconds()
	startTime := time.Now()

	for i := 0; i < calls; i++ {
		fn(i)
	}

	elapsedTime := time.Since(startTime)
	endCPU := readCPUSeconds()
	usedCPUPercent := cpuPercent(endCPU-startCPU, elapsedTime)

	// Runtime CPU metrics can be noisy on very short runs; take a longer sample for CPU%% only.
	if elapsedTime < 300*time.Millisecond {
		sampleCalls := calls
		sampleStartCPU := readCPUSeconds()
		sampleStart := time.Now()
		sampleElapsed := time.Duration(0)
		for sampleElapsed < time.Second {
			for i := 0; i < sampleCalls; i++ {
				fn(i)
			}
			sampleElapsed = time.Since(sampleStart)
			if sampleElapsed < time.Second {
				sampleCalls *= 2
			}
		}
		sampleEndCPU := readCPUSeconds()
		usedCPUPercent = cpuPercent(sampleEndCPU-sampleStartCPU, sampleElapsed)
	}
	// timeInNanoseconds := elapsedTime.Nanoseconds()

	fmt.Printf("%s took %vns and used %.2f percent of cpu\n\n", functionName, int(elapsedTime.Nanoseconds())/calls, usedCPUPercent)
}

// Benchmark profiles hot functions using benchmarkCalls iterations each.
func Benchmark() {
	fmt.Println("Starting profiling benchmarks...")
	fmt.Printf("Calls per function: %d\n\n", benchmarkCalls)
	fen, _ := chess.FEN("r1bqkbnr/pppp1ppp/2n5/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 0 3")
	game := chess.NewGame(fen)
	position := game.Position()
	pst := initPST()
	rootHash := fastPosHash(position)
	moves := position.ValidMoves()
	if len(moves) == 0 {
		fmt.Println("No legal moves in benchmark position; skipping profiling.")
		return
	}
	move := &moves[0]
	child := position.Update(move)
	bb, _ := position.MarshalBinary()
	wbb := bb[5]
	runBenchmark("MarshalBinary", benchmarkCalls, func(_ int) {
		bb, _ = position.MarshalBinary()

	})
	runBenchmark("PawnStructure", benchmarkCalls, func(_ int) {
		benchIntSink += pawnStructure(wbb)
	})
	runBenchmark("ValidMoves", benchmarkCalls, func(_ int) {
		benchIntSink += len(position.ValidMoves())
	})
	runBenchmark("Update", benchmarkCalls, func(_ int) {
		benchPosSink = position.Update(move)
	})
	runBenchmark("FastEvaluatePos", benchmarkCalls, func(_ int) {
		benchIntSink += EvaluatePos(position, &pst)
	})
	runBenchmark("EvaluateMove", benchmarkCalls, func(_ int) {
		benchIntSink += EvaluateMove(move, position, 6)
	})

	runBenchmark("fastPosHash", benchmarkCalls, func(_ int) {
		benchUint64Sink ^= fastPosHash(position)
	})

	runBenchmark("fastChildHash", benchmarkCalls, func(_ int) {
		benchUint64Sink ^= fastChildHash(position, child, move, rootHash)
	})
	runBenchmark("fastNullHash", benchmarkCalls, func(_ int) {
		benchUint64Sink ^= fastNullHash(position, child, rootHash)
	})
	runBenchmark("ttLookup", benchmarkCalls, func(_ int) {
		score, alpha, beta, ok := ttLookup(rootHash, 4, minScore, maxScore)
		benchIntSink += score + alpha + beta
		benchBoolSink = benchBoolSink != ok
	})

	runBenchmark("ttStore", benchmarkCalls, func(iter int) {
		key := rootHash + uint64(iter&7)
		ttStore(key, iter&1023, 4, ttBoundExact)
		benchIntSink += transpositionTable[key&ttMask].score
	})

	runBenchmark("quiescence_search_depth4", benchmarkCalls, func(_ int) {
		benchIntSink += quiescence_search(position, minScore, maxScore, true, 4, &pst, 0)
	})

	fmt.Println("Profiling benchmarks complete.")
}
