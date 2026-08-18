// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

// Package toolenv creates credential-free environments for release tooling.
package toolenv

import (
	"os"
	"strings"
)

var blockedNames = map[string]struct{}{
	"FEATBIT_ACCESS_TOKEN":          {},
	"FEATBIT_API_URL":               {},
	"FEATBIT_HTTP_TIMEOUT_SECONDS":  {},
	"FEATBIT_MCP_TOKEN":             {},
	"FEATBIT_MAX_CONCURRENCY":       {},
	"FEATBIT_MAX_RETRIES":           {},
	"FEATBIT_TEST_MEMBER_ID":        {},
	"FEATBIT_TEST_MEMBER_TOKEN":     {},
	"FEATBIT_TEST_ORGANIZATION_KEY": {},
	"FEATBIT_TEST_SERVICE_TOKEN":    {},
	"TF_ACC":                        {},
	"TF_CLI_CONFIG_FILE":            {},
	"TF_PLUGIN_CACHE_DIR":           {},
	"TF_REATTACH_PROVIDERS":         {},
	"TF_VAR_FEATBIT_TEST_MEMBER_ID": {},
}

// Sanitized returns the current process environment without FeatBit, live
// acceptance, Terraform override, cache, or reattachment settings. Additions
// replace same-named ambient variables case-insensitively.
func Sanitized(additions map[string]string) []string {
	blocked := make(map[string]struct{}, len(blockedNames)+len(additions))
	for name := range blockedNames {
		blocked[name] = struct{}{}
	}
	for name := range additions {
		blocked[strings.ToUpper(name)] = struct{}{}
	}

	environment := make([]string, 0, len(os.Environ())+len(additions))
	for _, item := range os.Environ() {
		name, _, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if _, remove := blocked[strings.ToUpper(name)]; remove {
			continue
		}
		environment = append(environment, item)
	}
	for name, value := range additions {
		environment = append(environment, name+"="+value)
	}
	return environment
}
