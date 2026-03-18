//go:build !windows

package main

import "runtime/metrics"

// readCPUSeconds returns total process CPU-seconds from runtime metrics.
func readCPUSeconds() float64 {
	samples := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}}
	metrics.Read(samples)
	if samples[0].Value.Kind() != metrics.KindFloat64 {
		return 0
	}
	return samples[0].Value.Float64()
}
