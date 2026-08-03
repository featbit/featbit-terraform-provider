// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"
)

type environmentLockManager struct {
	mu    sync.Mutex
	locks map[string]*environmentLockEntry
}

type environmentLockEntry struct {
	permit *semaphore.Weighted
	users  int
}

func newEnvironmentLockManager() *environmentLockManager {
	return &environmentLockManager{locks: make(map[string]*environmentLockEntry)}
}

func (m *environmentLockManager) acquire(
	ctx context.Context,
	projectID string,
	environmentID string,
) (func(), error) {
	key := strings.ToLower(projectID + "/" + environmentID)
	m.mu.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &environmentLockEntry{permit: semaphore.NewWeighted(1)}
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

func (m *environmentLockManager) releaseUser(key string, entry *environmentLockEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.users--
	if entry.users == 0 && m.locks[key] == entry {
		delete(m.locks, key)
	}
}
