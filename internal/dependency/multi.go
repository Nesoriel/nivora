package dependency

import (
	"context"
	"fmt"
)

// Checker is the shared readiness dependency surface.
type Checker interface {
	Check(context.Context) error
}

// Multi requires every configured dependency to be healthy.
type Multi struct {
	checkers []Checker
}

// New creates a composite dependency checker and skips nil entries.
func New(checkers ...Checker) *Multi {
	filtered := make([]Checker, 0, len(checkers))
	for _, checker := range checkers {
		if checker != nil {
			filtered = append(filtered, checker)
		}
	}
	return &Multi{checkers: filtered}
}

// Check fails on the first unavailable dependency.
func (m *Multi) Check(ctx context.Context) error {
	for index, checker := range m.checkers {
		if err := checker.Check(ctx); err != nil {
			return fmt.Errorf("dependency %d: %w", index+1, err)
		}
	}
	return nil
}
