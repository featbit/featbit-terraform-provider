// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"

	"golang.org/x/sync/semaphore"
)

type requestLimiter struct {
	permits *semaphore.Weighted
}

func newRequestLimiter(maxConcurrency int) *requestLimiter {
	return &requestLimiter{permits: semaphore.NewWeighted(int64(maxConcurrency))}
}

func (l *requestLimiter) acquire(ctx context.Context) error {
	return l.permits.Acquire(ctx, 1)
}

func (l *requestLimiter) release() {
	l.permits.Release(1)
}
