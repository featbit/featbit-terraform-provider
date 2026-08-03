// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"strings"

	"github.com/google/uuid"
)

const canonicalUUIDLength = 36

// CanonicalUUID parses value with google/uuid and returns its lowercase
// 8-4-4-4-12 representation. google/uuid intentionally accepts several
// alternate encodings, so the length and round-trip checks retain the
// provider's stricter public path/import contract.
func CanonicalUUID(value string) (string, bool) {
	if len(value) != canonicalUUIDLength {
		return "", false
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", false
	}
	canonical := parsed.String()
	if !strings.EqualFold(value, canonical) {
		return "", false
	}
	return canonical, true
}

// ValidUUID reports whether value has the UUID syntax accepted by documented
// FeatBit path parameters and Terraform import identifiers.
func ValidUUID(value string) bool {
	_, valid := CanonicalUUID(value)
	return valid
}

// EqualUUID compares two UUID strings without making hexadecimal letter case
// part of remote identity.
func EqualUUID(left, right string) bool {
	canonicalLeft, leftValid := CanonicalUUID(left)
	canonicalRight, rightValid := CanonicalUUID(right)
	return leftValid && rightValid && canonicalLeft == canonicalRight
}
