package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunEveryRepeatsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	code := runEvery(ctx, discardLogger(), time.Millisecond, func(context.Context) int {
		runs++
		if runs == 3 {
			cancel()
		}
		return 0
	})
	if code != 0 || runs != 3 {
		t.Errorf("code = %d, runs = %d; want 0, 3", code, runs)
	}
}

func TestRunEveryContinuesAfterFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	codes := []int{1, 1, 0}
	runs := 0
	runEvery(ctx, discardLogger(), time.Millisecond, func(context.Context) int {
		c := codes[runs]
		runs++
		if runs == len(codes) {
			cancel()
		}
		return c
	})
	if runs != len(codes) {
		t.Errorf("runs = %d, want %d: a failed run must not end the loop", runs, len(codes))
	}
}

func TestRunEveryStopsOnConfigError(t *testing.T) {
	runs := 0
	code := runEvery(context.Background(), discardLogger(), time.Millisecond, func(context.Context) int {
		runs++
		return 2
	})
	if code != 2 || runs != 1 {
		t.Errorf("code = %d, runs = %d; want 2, 1", code, runs)
	}
}

func TestRunEveryWaitsForInterval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runs := 0
	code := runEvery(ctx, discardLogger(), time.Hour, func(context.Context) int {
		runs++
		return 0
	})
	if code != 0 || runs != 1 {
		t.Errorf("code = %d, runs = %d; want 0, 1 (cancel during the wait)", code, runs)
	}
}
