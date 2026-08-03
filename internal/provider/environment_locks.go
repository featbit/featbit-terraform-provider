// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"sync"
)

type environmentLockManager struct {
	mu    sync.Mutex
	locks map[string]*environmentLockEntry
}

type environmentLockEntry struct {
	permit chan struct{}
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
		entry = &environmentLockEntry{permit: make(chan struct{}, 1)}
		m.locks[key] = entry
	}
	entry.users++
	m.mu.Unlock()

	select {
	case entry.permit <- struct{}{}:
		return func() {
			<-entry.permit
			m.releaseUser(key, entry)
		}, nil
	case <-ctx.Done():
		m.releaseUser(key, entry)
		return nil, ctx.Err()
	}
}

func (m *environmentLockManager) releaseUser(key string, entry *environmentLockEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.users--
	if entry.users == 0 && m.locks[key] == entry {
		delete(m.locks, key)
	}
}
