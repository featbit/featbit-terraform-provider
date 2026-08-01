// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	initialRetryDelay = 200 * time.Millisecond
	maximumRetryDelay = 5 * time.Second
)

type retryController struct {
	now    func() time.Time
	wait   func(context.Context, time.Duration) error
	jitter func(time.Duration) time.Duration
}

func newRetryController() retryController {
	return retryController{
		now:  time.Now,
		wait: waitForRetry,
		jitter: func(delay time.Duration) time.Duration {
			// Full-width multiplicative jitter in [0.5, 1.5) prevents provider
			// processes with the same configuration from retrying in lockstep.
			return time.Duration(float64(delay) * (0.5 + rand.Float64()))
		},
	}
}

func (r retryController) delay(response *http.Response, retryIndex int) time.Duration {
	if response != nil {
		if retryAfter, ok := ParseRetryAfter(response.Header.Get("Retry-After"), r.now()); ok {
			return retryAfter
		}
	}

	delay := initialRetryDelay
	for i := 0; i < retryIndex && delay < maximumRetryDelay; i++ {
		if delay > maximumRetryDelay/2 {
			delay = maximumRetryDelay
			break
		}
		delay *= 2
	}
	delay = r.jitter(delay)
	if delay > maximumRetryDelay {
		return maximumRetryDelay
	}
	if delay < 0 {
		return 0
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ShouldRetry permits retries only for safe GET requests and only for the
// transient classifications accepted by the provider contract.
func ShouldRetry(method string, classification Classification) bool {
	if method != http.MethodGet {
		return false
	}
	switch classification {
	case ClassificationRateLimited,
		ClassificationTransientServer,
		ClassificationTimeout,
		ClassificationNetwork:
		return true
	default:
		return false
	}
}

// ParseRetryAfter supports both delta-seconds and HTTP-date forms. Negative,
// past, malformed, and overflow values are rejected.
func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 || seconds > int64((time.Duration(1<<63-1))/time.Second) {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil || when.Before(now) {
		return 0, false
	}
	return when.Sub(now), true
}
