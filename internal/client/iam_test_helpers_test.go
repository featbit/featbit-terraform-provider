// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// iamTestResponse builds the common documented envelope for focused IAM
// adapter tests without coupling a fixture to one endpoint family.
func iamTestResponse(request *http.Request, status int, data any) *http.Response {
	success := status >= 200 && status < 300
	payload := map[string]any{
		"success": success,
		"data":    data,
		"errors":  []string{},
	}
	if !success {
		payload["errors"] = []string{"synthetic IAM failure"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return syntheticResponse(
		request,
		status,
		io.NopCloser(strings.NewReader(string(encoded))),
	)
}
