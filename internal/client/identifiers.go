// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"regexp"
	"strings"
)

var uuidSyntax = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidUUID reports whether value has the UUID syntax accepted by documented
// FeatBit path parameters and Terraform import identifiers.
func ValidUUID(value string) bool {
	return uuidSyntax.MatchString(value)
}

// EqualUUID compares two UUID strings without making hexadecimal letter case
// part of remote identity.
func EqualUUID(left, right string) bool {
	return strings.EqualFold(left, right)
}
