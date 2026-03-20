//go:build !windows

package main

import "runtime/metrics"

// Non-Windows CPU-time source used by profiling.go.
// readCPUSeconds returns total process CPU-seconds from runtime metrics.
func readCPUSeconds() float64 {
	samples := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return samples[0].Value.Float64()
}
