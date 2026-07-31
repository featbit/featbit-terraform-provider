package probe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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

type ExistenceDecision string

const (
	ExistencePresent       ExistenceDecision = "present"
	ExistenceAbsent        ExistenceDecision = "absent"
	ExistencePreserveState ExistenceDecision = "preserve_state"
)

func Classify(observation Observation, transportErr error) Classification {
	if transportErr != nil {
		if errors.Is(transportErr, context.Canceled) {
			return ClassificationCanceled
		}
		if errors.Is(transportErr, context.DeadlineExceeded) {
			return ClassificationTimeout
		}
		var netErr net.Error
		if errors.As(transportErr, &netErr) {
			if netErr.Timeout() {
				return ClassificationTimeout
			}
			return ClassificationNetwork
		}
		return ClassificationNetwork
	}

	switch observation.HTTPStatus {
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
	if observation.HTTPStatus >= 500 {
		return ClassificationTransientServer
	}
	if observation.HTTPStatus < 200 || observation.HTTPStatus >= 300 {
		return ClassificationAmbiguous
	}
	if observation.EnvelopeSuccess == nil {
		return ClassificationAmbiguous
	}
	if !*observation.EnvelopeSuccess {
		return ClassificationApplicationFailure
	}
	return ClassificationSuccess
}

// ResolveExistence removes state only after a successful exact scoped fallback
// proves zero matches. A direct 404 alone remains unconfirmed.
func ResolveExistence(direct Classification, fallbackCompleted bool, exactMatchCount int) ExistenceDecision {
	if direct == ClassificationSuccess {
		return ExistencePresent
	}
	if !fallbackCompleted {
		return ExistencePreserveState
	}
	switch exactMatchCount {
	case 0:
		return ExistenceAbsent
	case 1:
		return ExistencePresent
	default:
		return ExistencePreserveState
	}
}

func ShouldRetry(method string, classification Classification) bool {
	if method != http.MethodGet {
		return false
	}
	switch classification {
	case ClassificationRateLimited, ClassificationTransientServer, ClassificationTimeout, ClassificationNetwork:
		return true
	default:
		return false
	}
}

func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil || when.Before(now) {
		return 0, false
	}
	return when.Sub(now), true
}
