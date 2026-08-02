// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	redactedValue        = "<REDACTED>"
	redactedToken        = "<TOKEN>"
	redactedEmail        = "<MEMBER_EMAIL>"
	redactedTenant       = "<TENANT>"
	redactedPathIdentity = "<PATH_IDENTITY>"
)

var (
	emailPattern = regexp.MustCompile(
		`(?i)\b[A-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+\b`,
	)
	apiTokenPattern = regexp.MustCompile(`\bapi-[A-Za-z0-9_-]{8,}\b`)
	jwtPattern      = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	uuidPattern     = regexp.MustCompile(
		`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`,
	)
	apiPathPattern       = regexp.MustCompile(`(?i)/api/v1(?:/[^\s"'<>]*)?`)
	authorizationPattern = regexp.MustCompile(
		`(?i)(authorization\s*[:=]\s*)([^\s,;]+)`,
	)
	tenantResourcePattern = regexp.MustCompile(
		`(?i)\b(?:rn:)?featbit:[A-Za-z0-9_./:-]+`,
	)
)

type exactRedaction struct {
	value       string
	replacement string
}

// Redactor carries exact runtime secrets and identities without exposing them
// through formatting. It is immutable and safe to share between goroutines.
type Redactor struct {
	exact []exactRedaction
}

// NewRedactor registers exact credential/secret values.
func NewRedactor(secretValues ...string) *Redactor {
	redactor := &Redactor{}
	return redactor.withReplacement(redactedToken, secretValues...)
}

// With returns a copy that also removes exact runtime path identities.
func (r *Redactor) With(identityValues ...string) *Redactor {
	if r == nil {
		r = &Redactor{}
	}
	return r.withReplacement(redactedPathIdentity, identityValues...)
}

func (r *Redactor) withReplacement(replacement string, values ...string) *Redactor {
	combined := make([]exactRedaction, 0, len(r.exact)+len(values))
	combined = append(combined, r.exact...)
	seen := make(map[string]struct{}, len(combined)+len(values))
	for _, item := range combined {
		seen[item.value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		combined = append(combined, exactRedaction{value: value, replacement: replacement})
	}
	sort.SliceStable(combined, func(i, j int) bool {
		return len(combined[i].value) > len(combined[j].value)
	})
	return &Redactor{exact: combined}
}

// Format never reveals the registered values.
func (Redactor) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.Redactor{redacted}")
}

// Text removes exact registered values plus token, email, tenant resource,
// UUID, authorization, and API-path identity patterns.
func (r *Redactor) Text(input string) string {
	if r != nil {
		for _, item := range r.exact {
			input = strings.ReplaceAll(input, item.value, item.replacement)
		}
	}
	input = authorizationPattern.ReplaceAllString(input, `${1}`+redactedToken)
	input = apiTokenPattern.ReplaceAllString(input, redactedToken)
	input = jwtPattern.ReplaceAllString(input, redactedToken)
	input = emailPattern.ReplaceAllString(input, redactedEmail)
	input = tenantResourcePattern.ReplaceAllString(input, redactedTenant)
	input = apiPathPattern.ReplaceAllString(input, "/api/v1/"+redactedPathIdentity)
	input = uuidPattern.ReplaceAllString(input, redactedPathIdentity)
	return input
}

// RedactText applies shape-based redaction without registered exact values.
func RedactText(input string) string {
	return (&Redactor{}).Text(input)
}

// Headers returns a copy with credential headers removed and all other values
// passed through text redaction.
func (r *Redactor) Headers(headers http.Header) http.Header {
	redacted := make(http.Header, len(headers))
	for name, values := range headers {
		if sensitiveHeader(name) {
			redacted[name] = []string{redactedValue}
			continue
		}
		redactedValues := make([]string, len(values))
		for index, value := range values {
			redactedValues[index] = r.Text(value)
		}
		redacted[name] = redactedValues
	}
	return redacted
}

// Request returns diagnostic request metadata without credentials, tenant
// origins, query values, or exact API path identities.
func (r *Redactor) Request(request *http.Request) *http.Request {
	if request == nil {
		return nil
	}
	redacted := request.Clone(context.Background())
	redacted.Header = redactRequestHeaders(request.Header)
	redacted.Trailer = redactRequestHeaders(request.Trailer)
	redacted.Body = nil
	redacted.GetBody = nil
	redacted.ContentLength = 0
	redacted.TransferEncoding = nil
	redacted.Form = nil
	redacted.PostForm = nil
	redacted.MultipartForm = nil
	redacted.Host = "redacted.invalid"
	redacted.RemoteAddr = ""
	redacted.RequestURI = ""
	redacted.Pattern = ""
	redacted.Cancel = nil
	redacted.TLS = nil
	redacted.Response = nil
	if request.URL != nil {
		redacted.URL = &url.URL{
			Scheme: request.URL.Scheme,
			Host:   "redacted.invalid",
			Path:   "/api/v1/" + redactedPathIdentity,
		}
	}
	return redacted
}

func redactRequestHeaders(headers http.Header) http.Header {
	redacted := make(http.Header, len(headers))
	for name, values := range headers {
		redactedValues := make([]string, len(values))
		for index := range values {
			redactedValues[index] = redactedValue
		}
		redacted[name] = redactedValues
	}
	return redacted
}

func sensitiveHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "set-cookie", "proxy-authorization", "x-api-key":
		return true
	default:
		return false
	}
}
