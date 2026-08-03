// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
)

// keyedLockManager provides cancellation-safe, reference-counted one-writer
// serialization for the narrow multi-call lifecycles that supply their own
// canonical identity key.
type keyedLockManager struct {
	mu    sync.Mutex
	locks map[string]*keyedLockEntry
}

type keyedLockEntry struct {
	permit *semaphore.Weighted
	users  int
}

func newKeyedLockManager() *keyedLockManager {
	return &keyedLockManager{locks: make(map[string]*keyedLockEntry)}
}

func (m *keyedLockManager) acquire(
	ctx context.Context,
	key string,
) (func(), error) {
	m.mu.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &keyedLockEntry{permit: semaphore.NewWeighted(1)}
		m.locks[key] = entry
	}
	entry.users++
	m.mu.Unlock()

	if err := entry.permit.Acquire(ctx, 1); err != nil {
		m.releaseUser(key, entry)
		return nil, err
	}
	return func() {
		entry.permit.Release(1)
		m.releaseUser(key, entry)
	}, nil
}

func (m *keyedLockManager) releaseUser(key string, entry *keyedLockEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.users--
	if entry.users == 0 && m.locks[key] == entry {
		delete(m.locks, key)
	}
}
