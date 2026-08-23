// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

// resolveExactOne owns the common exact zero/one/duplicate contract for an
// already complete and endpoint-validated collection. Endpoint callers retain
// identity comparison, operation naming, and sensitive-value redaction.
func resolveExactOne[T any](
	items []T,
	matches func(T) bool,
	operation string,
	errorRedactor *Redactor,
) (T, bool, error) {
	var match T
	matchCount := 0
	for _, item := range items {
		if matches(item) {
			match = item
			matchCount++
		}
	}

	switch matchCount {
	case 0:
		return match, false, nil
	case 1:
		return match, true, nil
	default:
		var zero T
		return zero, false, newAPIError(
			ClassificationAmbiguous,
			0,
			operation,
			nil,
			errorRedactor,
		)
	}
}
