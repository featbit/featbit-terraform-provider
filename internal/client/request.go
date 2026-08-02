// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) newRequest(
	ctx context.Context,
	method string,
	segments []string,
	body io.Reader,
) (*http.Request, error) {
	endpoint := c.endpointURL(segments...)
	return http.NewRequestWithContext(ctx, method, endpoint.String(), body)
}

func (c *Client) newJSONRequest(
	ctx context.Context,
	method string,
	segments []string,
	payload any,
) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := c.newRequest(ctx, method, segments, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

// endpointURL preserves every input as one escaped path component. Endpoint
// adapters still validate UUID components before calling it.
func (c *Client) endpointURL(segments ...string) url.URL {
	endpoint := c.baseURL
	plainPath := strings.TrimSuffix(endpoint.Path, "/")
	escapedPath := strings.TrimSuffix(endpoint.EscapedPath(), "/")
	for _, segment := range segments {
		plainPath += "/" + segment
		escapedPath += "/" + url.PathEscape(segment)
	}
	endpoint.Path = plainPath
	if escapedPath != plainPath {
		endpoint.RawPath = escapedPath
	} else {
		endpoint.RawPath = ""
	}
	return endpoint
}
