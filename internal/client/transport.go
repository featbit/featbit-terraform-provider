// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"
)

var contextHeaders = []string{
	"Organization",
	"Workspace",
	"X-Organization",
	"X-Organization-Id",
	"X-Workspace",
	"X-Workspace-Id",
}

type authorizationTransport struct {
	base      http.RoundTripper
	apiScheme string
	apiHost   string
	apiPath   string
	token     string
	userAgent string
}

// Format prevents accidental formatting of the transport from exposing the
// Authorization credential or configured API origin.
func (authorizationTransport) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "client.authorizationTransport{redacted}")
}

func (t *authorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("FeatBit API request URL is missing")
	}
	if request.URL.User != nil ||
		!strings.EqualFold(request.URL.Scheme, t.apiScheme) ||
		!strings.EqualFold(request.URL.Host, t.apiHost) ||
		!withinAPIPath(request.URL.EscapedPath(), t.apiPath) {
		return nil, errRequestBoundary
	}

	requestCopy := request.Clone(request.Context())
	requestCopy.Header = request.Header.Clone()
	requestCopy.Header.Set("Authorization", t.token)
	requestCopy.Header.Set("User-Agent", t.userAgent)
	for _, header := range contextHeaders {
		requestCopy.Header.Del(header)
	}

	return t.base.RoundTrip(requestCopy)
}

func withinAPIPath(requestPath, apiPath string) bool {
	return requestPath == apiPath || strings.HasPrefix(requestPath, apiPath+"/")
}

func makeUserAgent(providerVersion string) string {
	providerVersion = strings.TrimSpace(providerVersion)
	if providerVersion == "" || strings.IndexFunc(providerVersion, func(r rune) bool {
		return r > unicode.MaxASCII || unicode.IsControl(r) || unicode.IsSpace(r) ||
			strings.ContainsRune("()<>@,;:\\\"/[]?={}", r)
	}) != -1 {
		providerVersion = "dev"
	}
	return userAgentProduct + "/" + providerVersion
}
