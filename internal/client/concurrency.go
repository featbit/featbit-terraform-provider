// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import "context"

type requestLimiter struct {
	permits chan struct{}
}

func newRequestLimiter(maxConcurrency int) *requestLimiter {
	return &requestLimiter{permits: make(chan struct{}, maxConcurrency)}
}

func (l *requestLimiter) acquire(ctx context.Context) error {
	select {
	case l.permits <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *requestLimiter) release() {
	<-l.permits
}
