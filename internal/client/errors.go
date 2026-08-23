// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode"
)

// Classification is the centralized transport/envelope error class used by
// resource lifecycle code. Not-found remains unconfirmed until an exact
// parent-scoped fallback completes.
type Classification string

const (
	ClassificationSuccess             Classification = "success"
	ClassificationValidation          Classification = "validation"
	ClassificationAuthentication      Classification = "authentication"
	ClassificationAuthorization       Classification = "authorization"
	ClassificationNotFoundUnconfirmed Classification = "not_found_unconfirmed"
	ClassificationConflict            Classification = "conflict"
	ClassificationRateLimited         Classification = "rate_limited"
	ClassificationTransientServer     Classification = "transient_server"
	ClassificationApplicationFailure  Classification = "application_failure"
	ClassificationTimeout             Classification = "timeout"
	ClassificationCanceled            Classification = "canceled"
	ClassificationNetwork             Classification = "network"
	ClassificationAmbiguous           Classification = "ambiguous"
)

var (
	errRequestBoundary  = errors.New("request is outside the FeatBit API boundary")
	errResponseTooLarge = errors.New("FeatBit API response exceeds the supported size")
)

// APIError is a redaction-safe error returned by the handwritten client.
// Server details are retained only after redaction.
type APIError struct {
	classification Classification
	statusCode     int
	operation      string
	details        []string
	cause          error
}

// Error intentionally omits server messages, URLs, and remote identities.
func (e *APIError) Error() string {
	if e == nil {
		return "FeatBit API operation failed"
	}
	if e.statusCode != 0 {
		return fmt.Sprintf(
			"FeatBit API operation %s failed (%s, HTTP %d)",
			e.operation,
			e.classification,
			e.statusCode,
		)
	}
	return fmt.Sprintf(
		"FeatBit API operation %s failed (%s)",
		e.operation,
		e.classification,
	)
}

// Format uses the same already-redacted representation for every verb.
func (e *APIError) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, e.Error())
}

// Unwrap exposes only cancellation/deadline sentinels; raw network errors are
// discarded because they may contain request URLs and runtime identities.
func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Classification reports the stable provider error class.
func (e *APIError) Classification() Classification {
	if e == nil {
		return ClassificationAmbiguous
	}
	return e.classification
}

// StatusCode reports the HTTP status, or zero for a transport failure.
func (e *APIError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.statusCode
}

// Details returns a copy of already-redacted FeatBit envelope errors.
func (e *APIError) Details() []string {
	if e == nil {
		return nil
	}
	return append([]string(nil), e.details...)
}

func newAPIError(
	classification Classification,
	statusCode int,
	operation string,
	details []string,
	redactor *Redactor,
) *APIError {
	operation = safeOperationName(operation)
	redactedDetails := make([]string, 0, len(details))
	for _, detail := range details {
		if redactor != nil {
			detail = redactor.Text(detail)
		} else {
			detail = RedactText(detail)
		}
		redactedDetails = append(redactedDetails, detail)
	}
	return &APIError{
		classification: classification,
		statusCode:     statusCode,
		operation:      operation,
		details:        redactedDetails,
	}
}

func newTransportError(err error) *APIError {
	var apiError *APIError
	if errors.As(err, &apiError) {
		return apiError
	}
	classification := Classify(0, nil, err)
	apiError = newAPIError(classification, 0, "request", nil, nil)
	if errors.Is(err, context.Canceled) {
		apiError.cause = context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		apiError.cause = context.DeadlineExceeded
	}
	return apiError
}

// readErrorWithoutDetails retains classification, status, and cancellation
// sentinels while discarding server detail strings that a caller cannot
// exhaustively enumerate and redact.
func readErrorWithoutDetails(operation string, err error, redactor *Redactor) error {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return newTransportError(err)
	}
	sanitized := newAPIError(
		apiError.Classification(),
		apiError.StatusCode(),
		operation,
		nil,
		redactor,
	)
	sanitized.cause = apiError.cause
	return sanitized
}

func safeOperationName(operation string) string {
	if operation == "" || strings.IndexFunc(operation, func(r rune) bool {
		return r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '_' || r == '-' || r == '.')
	}) != -1 {
		return "request"
	}
	return operation
}

// Classify combines transport errors, HTTP status, and envelope success. HTTP
// 401/403 always mean authentication/authorization, and a 2xx success=false is
// always an application failure.
func Classify(statusCode int, envelopeSuccess *bool, transportErr error) Classification {
	if transportErr != nil {
		var apiError *APIError
		if errors.As(transportErr, &apiError) {
			return apiError.Classification()
		}
		if errors.Is(transportErr, context.Canceled) {
			return ClassificationCanceled
		}
		if errors.Is(transportErr, context.DeadlineExceeded) {
			return ClassificationTimeout
		}
		if errors.Is(transportErr, errRequestBoundary) || errors.Is(transportErr, errResponseTooLarge) {
			return ClassificationAmbiguous
		}
		var networkError net.Error
		if errors.As(transportErr, &networkError) && networkError.Timeout() {
			return ClassificationTimeout
		}
		return ClassificationNetwork
	}

	switch statusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return ClassificationValidation
	case http.StatusUnauthorized:
		return ClassificationAuthentication
	case http.StatusForbidden:
		return ClassificationAuthorization
	case http.StatusNotFound:
		return ClassificationNotFoundUnconfirmed
	case http.StatusConflict:
		return ClassificationConflict
	case http.StatusTooManyRequests:
		return ClassificationRateLimited
	}
	if statusCode >= http.StatusInternalServerError && statusCode < 600 {
		return ClassificationTransientServer
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return ClassificationAmbiguous
	}
	if envelopeSuccess == nil {
		return ClassificationAmbiguous
	}
	if !*envelopeSuccess {
		return ClassificationApplicationFailure
	}
	return ClassificationSuccess
}

type responseEnvelope struct {
	Data    json.RawMessage `json:"data"`
	Errors  json.RawMessage `json:"errors"`
	Success *bool           `json:"success"`
}

// DecodeResponse centrally validates the FeatBit response envelope and
// decodes only its data member. Additional sensitive values let callers redact
// exact path keys that are not recognizable by shape alone.
func (c *Client) DecodeResponse(
	operation string,
	response *http.Response,
	destination any,
	sensitiveValues ...string,
) error {
	if response == nil {
		return newAPIError(ClassificationAmbiguous, 0, operation, nil, c.redactor)
	}

	body, err := readBoundedResponse(response.Body)
	if err != nil {
		return newTransportError(err)
	}
	redactor := c.redactor.With(sensitiveValues...)

	var envelope responseEnvelope
	decodeError := json.Unmarshal(body, &envelope)
	classification := Classify(response.StatusCode, envelope.Success, nil)
	if decodeError != nil {
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			classification = ClassificationAmbiguous
		}
		return newAPIError(classification, response.StatusCode, operation, nil, redactor)
	}

	details := decodeEnvelopeErrors(envelope.Errors)
	if classification != ClassificationSuccess {
		return newAPIError(classification, response.StatusCode, operation, details, redactor)
	}
	if destination == nil {
		return nil
	}
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, destination) != nil {
		return newAPIError(
			ClassificationAmbiguous,
			response.StatusCode,
			operation,
			nil,
			redactor,
		)
	}
	return nil
}

func decodeEnvelopeErrors(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var details []string
	if json.Unmarshal(raw, &details) == nil {
		return details
	}
	var detail string
	if json.Unmarshal(raw, &detail) == nil {
		return []string{detail}
	}
	return nil
}
