package main

import (
	"context"
	"time"
)

// SearchControl carries cancellation and time-budget state for one active search.
type SearchControl struct {
	ctx    context.Context
	cancel context.CancelFunc

	enabled      bool
	softDeadline time.Time
	hardDeadline time.Time
}

// NewSearchControl builds cancellation/deadline control for a search from UCI options.
func NewSearchControl(options *SearchOptions, whiteToMove bool) *SearchControl {
	ctx, cancel := context.WithCancel(context.Background())
	control := &SearchControl{ctx: ctx, cancel: cancel}
	if options == nil || options.isInf {
		return control
	}

	budgetMs := computeMoveBudgetMs(options, whiteToMove)
	if budgetMs <= 0 {
		return control
	}

	now := time.Now()
	softMs := (budgetMs * timeSoftBudgetPercent) / 100
	if softMs < timeControlMinThinkMs {
		softMs = timeControlMinThinkMs
	}
	if softMs >= budgetMs {
		softMs = budgetMs - 1
	}
	if softMs < 1 {
		softMs = 1
	}

	control.enabled = true
	control.softDeadline = now.Add(time.Duration(softMs) * time.Millisecond)
	control.hardDeadline = now.Add(time.Duration(budgetMs) * time.Millisecond)
	return control
}

func computeMoveBudgetMs(options *SearchOptions, whiteToMove bool) int {
	if options.moveTime > 0 {
		budget := options.moveTime - timeControlSafetyBufferMs
		if budget < 1 {
			budget = 1
		}
		return budget
	}

	remaining := options.blackTime
	increment := options.binc
	if whiteToMove {
		remaining = options.whiteTime
		increment = options.winc
	}
	if remaining <= 0 {
		return 0
	}

	movesToGo := options.movesToGo
	if movesToGo <= 0 {
		movesToGo = timeDefaultMovesToGo
	}

	base := remaining / movesToGo
	bonus := (increment * 3) / 4
	budget := base + bonus

	maxBudget := remaining - timeControlSafetyBufferMs
	if maxBudget < 1 {
		maxBudget = 1
	}

	minBudget := timeControlMinThinkMs
	if minBudget > maxBudget {
		minBudget = maxBudget
	}

	if budget < minBudget {
		budget = minBudget
	}
	if budget > maxBudget {
		budget = maxBudget
	}

	return budget
}

func (s *SearchControl) Cancel() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *SearchControl) Done() <-chan struct{} {
	if s == nil || s.ctx == nil {
		return nil
	}
	return s.ctx.Done()
}

func (s *SearchControl) isCanceled() bool {
	if s == nil || s.ctx == nil {
		return false
	}
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

func (s *SearchControl) shouldStopBeforeDepth(hasCompletedDepth bool) bool {
	if s == nil {
		return false
	}
	if s.isCanceled() {
		return true
	}
	if !s.enabled {
		return false
	}

	now := time.Now()
	if !now.Before(s.hardDeadline) {
		s.Cancel()
		return true
	}
	if hasCompletedDepth && !now.Before(s.softDeadline) {
		return true
	}
	return false
}

func (s *SearchControl) shouldStopInSearch(nodeCount int) bool {
	if s == nil {
		return false
	}
	if nodeCount&searchStopCheckMask != 0 {
		return false
	}
	if s.isCanceled() {
		return true
	}
	if s.enabled && !time.Now().Before(s.hardDeadline) {
		s.Cancel()
		return true
	}
	return false
}
