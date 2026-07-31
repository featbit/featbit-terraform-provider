package probe

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClassifyStatusEnvelopeAndTransport(t *testing.T) {
	t.Parallel()

	success := true
	failure := false
	tests := map[string]struct {
		observation Observation
		err         error
		want        Classification
	}{
		"success":          {observation: Observation{HTTPStatus: 200, EnvelopeSuccess: &success}, want: ClassificationSuccess},
		"success false":    {observation: Observation{HTTPStatus: 200, EnvelopeSuccess: &failure}, want: ClassificationApplicationFailure},
		"missing envelope": {observation: Observation{HTTPStatus: 200}, want: ClassificationAmbiguous},
		"validation":       {observation: Observation{HTTPStatus: 400}, want: ClassificationValidation},
		"authentication":   {observation: Observation{HTTPStatus: 401}, want: ClassificationAuthentication},
		"authorization":    {observation: Observation{HTTPStatus: 403}, want: ClassificationAuthorization},
		"not found":        {observation: Observation{HTTPStatus: 404}, want: ClassificationNotFoundUnconfirmed},
		"conflict":         {observation: Observation{HTTPStatus: 409}, want: ClassificationConflict},
		"rate limited":     {observation: Observation{HTTPStatus: 429}, want: ClassificationRateLimited},
		"server":           {observation: Observation{HTTPStatus: 503}, want: ClassificationTransientServer},
		"canceled":         {err: context.Canceled, want: ClassificationCanceled},
		"timeout":          {err: context.DeadlineExceeded, want: ClassificationTimeout},
		"network":          {err: errors.New("synthetic network failure"), want: ClassificationNetwork},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(test.observation, test.err); got != test.want {
				t.Fatalf("Classify() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestResolveExistencePreservesStateUntilExactFallback(t *testing.T) {
	t.Parallel()

	if got := ResolveExistence(ClassificationNotFoundUnconfirmed, false, 0); got != ExistencePreserveState {
		t.Fatalf("direct 404 decision = %s", got)
	}
	if got := ResolveExistence(ClassificationNotFoundUnconfirmed, true, 0); got != ExistenceAbsent {
		t.Fatalf("exact zero decision = %s", got)
	}
	if got := ResolveExistence(ClassificationNotFoundUnconfirmed, true, 1); got != ExistencePresent {
		t.Fatalf("exact one decision = %s", got)
	}
	if got := ResolveExistence(ClassificationNotFoundUnconfirmed, true, 2); got != ExistencePreserveState {
		t.Fatalf("duplicate decision = %s", got)
	}
	if got := ResolveExistence(ClassificationAuthentication, false, 0); got != ExistencePreserveState {
		t.Fatalf("authentication decision = %s", got)
	}
}

func TestRetryOnlySafeReadsAndParseRetryAfter(t *testing.T) {
	t.Parallel()

	for _, classification := range []Classification{
		ClassificationRateLimited,
		ClassificationTransientServer,
		ClassificationTimeout,
		ClassificationNetwork,
	} {
		if !ShouldRetry(http.MethodGet, classification) {
			t.Fatalf("GET should retry %s", classification)
		}
		if ShouldRetry(http.MethodPost, classification) {
			t.Fatalf("POST must not retry %s", classification)
		}
	}
	if ShouldRetry(http.MethodGet, ClassificationCanceled) ||
		ShouldRetry(http.MethodGet, ClassificationAuthentication) ||
		ShouldRetry(http.MethodGet, ClassificationConflict) {
		t.Fatal("unsafe retry classification was accepted")
	}

	now := time.Date(2026, 7, 30, 7, 0, 0, 0, time.UTC)
	if got, ok := ParseRetryAfter("5", now); !ok || got != 5*time.Second {
		t.Fatalf("Retry-After seconds = %s, %t", got, ok)
	}
	if got, ok := ParseRetryAfter(now.Add(10*time.Second).Format(http.TimeFormat), now); !ok || got != 10*time.Second {
		t.Fatalf("Retry-After date = %s, %t", got, ok)
	}
	if _, ok := ParseRetryAfter("invalid", now); ok {
		t.Fatal("invalid Retry-After accepted")
	}
}
