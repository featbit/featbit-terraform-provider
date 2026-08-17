// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

// completePageTracker owns the shared consistency rules for callers that must
// consume a complete paginated collection. Endpoint-specific requests, wire
// validation, conversion, and sensitive values remain with each caller.
type completePageTracker struct {
	operation       string
	pageSize        int
	maxPageIndex    int64
	expectedTotal   int64
	collected       int64
	seenExactIDs    map[string]struct{}
	defaultRedactor *Redactor
}

func newCompletePageTracker(
	operation string,
	pageSize int,
	maxPageIndex int64,
	redactor *Redactor,
) *completePageTracker {
	return &completePageTracker{
		operation:       operation,
		pageSize:        pageSize,
		maxPageIndex:    maxPageIndex,
		expectedTotal:   -1,
		seenExactIDs:    make(map[string]struct{}),
		defaultRedactor: redactor,
	}
}

func (t *completePageTracker) validatePage(
	totalCount *int64,
	itemsPresent bool,
	itemCount int,
	statusCode int,
) error {
	if totalCount == nil || *totalCount < 0 || !itemsPresent || itemCount > t.pageSize {
		return t.ambiguousError(statusCode, t.defaultRedactor)
	}
	if t.expectedTotal < 0 {
		t.expectedTotal = *totalCount
	} else if *totalCount != t.expectedTotal {
		return t.ambiguousError(statusCode, t.defaultRedactor)
	}
	if itemCount == 0 && t.collected < t.expectedTotal {
		return t.ambiguousError(statusCode, t.defaultRedactor)
	}
	return nil
}

func (t *completePageTracker) recordExactID(
	canonicalID string,
	statusCode int,
	duplicateRedactor *Redactor,
) error {
	if _, duplicate := t.seenExactIDs[canonicalID]; duplicate {
		return t.ambiguousError(statusCode, duplicateRedactor)
	}
	t.seenExactIDs[canonicalID] = struct{}{}
	t.collected++
	return nil
}

func (t *completePageTracker) pageComplete(
	pageIndex int64,
	statusCode int,
) (bool, error) {
	if t.collected > t.expectedTotal {
		return false, t.ambiguousError(statusCode, t.defaultRedactor)
	}
	if t.collected == t.expectedTotal {
		return true, nil
	}
	if pageIndex == t.maxPageIndex {
		return false, t.ambiguousError(statusCode, t.defaultRedactor)
	}
	return false, nil
}

func (t *completePageTracker) ambiguousError(
	statusCode int,
	redactor *Redactor,
) error {
	return newAPIError(
		ClassificationAmbiguous,
		statusCode,
		t.operation,
		nil,
		redactor,
	)
}
