package main

import (
	"context"
	"log/slog"
	"time"
)

// minInterval is the shortest PASCHA_INTERVAL fetch accepts. Famly is
// read at the rate a human would; a tighter loop buys nothing, since
// educators post a handful of times a day.
const minInterval = 15 * time.Minute

// runEvery calls once at start and then every interval, measured
// from the start of the previous run, until ctx is cancelled. A run
// that returns 1 (transport or upload failure) is logged and retried
// on the next tick; a run that returns 2 is a configuration error and
// ends the loop with that code. Cancellation returns 0 so `docker
// stop` reads as a clean shutdown.
func runEvery(ctx context.Context, logger *slog.Logger, interval time.Duration, once func(context.Context) int) int {
	for {
		start := time.Now()
		code := once(ctx)
		if ctx.Err() != nil {
			logger.Info("fetch loop stopped")
			return 0
		}
		if code == 2 {
			return code
		}
		next := start.Add(interval)
		if code != 0 {
			logger.Warn("fetch failed; retrying at next interval", "code", code, "next", next.Format(time.RFC3339))
		} else {
			logger.Info("next fetch", "at", next.Format(time.RFC3339))
		}
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			logger.Info("fetch loop stopped")
			return 0
		case <-t.C:
		}
	}
}
