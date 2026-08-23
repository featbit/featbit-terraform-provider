// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import "github.com/featbit/terraform-provider-featbit/internal/client"

func mutationOutcomeAmbiguous(err error) bool {
	switch client.Classify(0, nil, err) {
	case client.ClassificationRateLimited,
		client.ClassificationTransientServer,
		client.ClassificationTimeout,
		client.ClassificationCanceled,
		client.ClassificationNetwork,
		client.ClassificationAmbiguous:
		return true
	default:
		return false
	}
}

func mutationNeedsReconciliation(err error) bool {
	if mutationOutcomeAmbiguous(err) {
		return true
	}
	switch client.Classify(0, nil, err) {
	case client.ClassificationConflict, client.ClassificationNotFoundUnconfirmed:
		return true
	default:
		return false
	}
}
