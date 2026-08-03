// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	featureFlagIDOne        = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	featureFlagIDTwo        = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	featureFlagIDThree      = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	featureFlagVariationOne = "12345678-1234-4234-8234-1234567890ab"
	featureFlagVariationTwo = "abcdefab-cdef-4abc-8def-abcdefabcdef"
)

func TestGetFeatureFlagDirectContractAndSafeWireShape(t *testing.T) {
	t.Parallel()

	const (
		key                  = "flag/key with space"
		variationValue       = "synthetic-definition-value"
		unsafeTag            = "synthetic-ui-owned-tag"
		unsafeTarget         = "synthetic-ui-owned-target"
		unsafeFallthroughKey = "synthetic-ui-owned-fallthrough"
	)
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodGet {
				t.Fatalf("method = %q, want GET", request.Method)
			}
			wantPath := "/api/v1/envs/" + environmentOne +
				"/feature-flags/flag%2Fkey%20with%20space"
			if got := request.URL.EscapedPath(); got != wantPath {
				t.Fatalf("escaped path = %q, want %q", got, wantPath)
			}
			if request.URL.RawQuery != "" {
				t.Fatalf("unexpected query = %q", request.URL.RawQuery)
			}
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("exact GET unexpectedly contained a body")
			}
			if request.Header.Get("Authorization") != syntheticAccessToken {
				t.Fatal("request did not use direct access-token authorization")
			}
			if request.Header.Get("User-Agent") != "terraform-provider-featbit/test" {
				t.Fatal("request did not use the provider User-Agent")
			}
			for _, header := range contextHeaders {
				if request.Header.Get(header) != "" {
					t.Fatalf("request sent unsupported context header %q", header)
				}
			}

			data := `{"id":"` + featureFlagIDOne +
				`","envId":"` + environmentOne +
				`","name":"Flag","description":"Definition","key":"` + key +
				`","variationType":"string","variations":[` +
				`{"id":"` + featureFlagVariationOne +
				`","name":"First","value":"` + variationValue + `"}],` +
				`"isArchived":false,"isEnabled":true,"disabledVariationId":"` +
				featureFlagVariationOne + `","tags":["` + unsafeTag + `"],` +
				`"targetUsers":[{"keyIds":["` + unsafeTarget + `"]}],` +
				`"rules":[{"name":"` + unsafeTarget + `"}],` +
				`"fallthrough":{"dispatchKey":"` + unsafeFallthroughKey + `"}}`
			return featureFlagTestResponse(request, http.StatusOK, data), nil
		},
	))

	flag, status, err := clientUnderTest.GetFeatureFlag(context.Background(), environmentOne, key)
	if err != nil {
		t.Fatalf("GetFeatureFlag() error = %v", err)
	}
	if status != FeatureFlagStatusActive || flag.ID != featureFlagIDOne ||
		flag.EnvironmentID != environmentOne || flag.Key != key || flag.IsArchived ||
		len(flag.Variations) != 1 || flag.Variations[0].Value != variationValue {
		t.Fatal("GetFeatureFlag() did not return the exact safe definition")
	}
	encoded, err := json.Marshal(flag)
	if err != nil {
		t.Fatal("json.Marshal(FeatureFlag) failed")
	}
	for _, unsafe := range []string{
		"isEnabled",
		"disabledVariationId",
		"tags",
		"targetUsers",
		"rules",
		"fallthrough",
		unsafeTag,
		unsafeTarget,
		unsafeFallthroughKey,
	} {
		if strings.Contains(string(encoded), unsafe) {
			t.Fatal("safe Feature Flag wire model retained a UI-owned field")
		}
	}
	formatted := fmt.Sprintf(
		"%v|%+v|%#v|%v|%+v|%#v",
		flag,
		flag,
		flag,
		flag.Variations[0],
		flag.Variations[0],
		flag.Variations[0],
	)
	for _, unsafe := range []string{
		featureFlagIDOne,
		environmentOne,
		key,
		featureFlagVariationOne,
		variationValue,
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted Feature Flag response exposed a runtime identity or value")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d, want 1", calls.Load())
	}
}

func TestListFeatureFlagsConsumesEveryPageAndTerminatesAtTotalCount(t *testing.T) {
	t.Parallel()

	items := [][]string{
		{
			featureFlagListItemTestJSON(featureFlagIDOne, "first", "one"),
			featureFlagListItemTestJSON(featureFlagIDTwo, "second", "two"),
		},
		{
			featureFlagListItemTestJSON(featureFlagIDThree, "third", "three"),
		},
	}
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			call := int(calls.Add(1))
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/feature-flags" {
				t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
			}
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("collection GET unexpectedly contained a body")
			}
			query := request.URL.Query()
			if len(query) != 3 || query.Get("IsArchived") != "true" ||
				query.Get("PageSize") != strconv.Itoa(featureFlagPageSize) ||
				query.Get("PageIndex") != strconv.Itoa(call-1) {
				t.Fatalf("unexpected documented query = %q", request.URL.RawQuery)
			}
			if request.URL.RawQuery != "IsArchived=true&PageIndex="+strconv.Itoa(call-1)+
				"&PageSize="+strconv.Itoa(featureFlagPageSize) {
				t.Fatalf("raw query = %q", request.URL.RawQuery)
			}
			for _, header := range contextHeaders {
				if request.Header.Get(header) != "" {
					t.Fatalf("request sent unsupported context header %q", header)
				}
			}
			if call > len(items) {
				t.Fatal("pagination did not terminate at totalCount")
			}
			data := featureFlagPageTestJSON(3, items[call-1])
			return featureFlagTestResponse(request, http.StatusOK, data), nil
		},
	))

	flags, err := clientUnderTest.ListFeatureFlags(context.Background(), environmentOne, true)
	if err != nil {
		t.Fatalf("ListFeatureFlags() error = %v", err)
	}
	if len(flags) != 3 || calls.Load() != 2 {
		t.Fatalf("complete collection size/calls = %d/%d, want 3/2", len(flags), calls.Load())
	}
	for _, flag := range flags {
		if flag.EnvironmentID != environmentOne || !flag.IsArchived {
			t.Fatal("collection context was not applied to an archived list item")
		}
	}
}

func TestListFeatureFlagsFailsClosedForMalformedOrIncompletePages(t *testing.T) {
	t.Parallel()

	first := featureFlagListItemTestJSON(featureFlagIDOne, "first", "first")
	second := featureFlagListItemTestJSON(featureFlagIDTwo, "second", "second")
	oversizedItems := make([]string, 0, featureFlagPageSize+1)
	for index := 0; index <= featureFlagPageSize; index++ {
		id := uuid.NewSHA1(
			uuid.NameSpaceURL,
			[]byte(fmt.Sprintf("feature-flag-page-item/v1/%d", index)),
		).String()
		oversizedItems = append(
			oversizedItems,
			featureFlagListItemTestJSON(id, "oversized", fmt.Sprintf("key-%d", index)),
		)
	}

	tests := map[string]struct {
		archived           bool
		pages              []string
		wantCount          int
		wantClassification Classification
	}{
		"empty complete collection": {
			pages:     []string{featureFlagPageTestJSON(0, []string{})},
			wantCount: 0,
		},
		"null data": {
			pages:              []string{"null"},
			wantClassification: ClassificationAmbiguous,
		},
		"missing total count": {
			pages:              []string{`{"items":[]}`},
			wantClassification: ClassificationAmbiguous,
		},
		"negative total count": {
			pages:              []string{`{"totalCount":-1,"items":[]}`},
			wantClassification: ClassificationAmbiguous,
		},
		"null items": {
			pages:              []string{`{"totalCount":0,"items":null}`},
			wantClassification: ClassificationAmbiguous,
		},
		"empty page before total": {
			pages: []string{
				featureFlagPageTestJSON(2, []string{first}),
				featureFlagPageTestJSON(2, []string{}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"total count changes": {
			pages: []string{
				featureFlagPageTestJSON(2, []string{first}),
				featureFlagPageTestJSON(3, []string{second}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"items exceed total": {
			pages: []string{
				featureFlagPageTestJSON(1, []string{first, second}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"page exceeds requested size": {
			pages: []string{
				featureFlagPageTestJSON(int64(len(oversizedItems)), oversizedItems),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"repeated item across pages": {
			pages: []string{
				featureFlagPageTestJSON(2, []string{first}),
				featureFlagPageTestJSON(2, []string{first}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"invalid item ID": {
			pages: []string{
				featureFlagPageTestJSON(1, []string{
					featureFlagListItemTestJSON("not-a-uuid", "invalid", "invalid"),
				}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"missing item key": {
			pages: []string{
				featureFlagPageTestJSON(1, []string{
					`{"id":"` + featureFlagIDOne + `","name":"Missing","key":""}`,
				}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"wrong environment context": {
			pages: []string{
				featureFlagPageTestJSON(1, []string{
					featureFlagExactTestJSON(featureFlagIDOne, environmentTwo, "wrong", false),
				}),
			},
			wantClassification: ClassificationAmbiguous,
		},
		"conflicting archive context": {
			archived: true,
			pages: []string{
				featureFlagPageTestJSON(1, []string{
					featureFlagExactTestJSON(featureFlagIDOne, environmentOne, "wrong", false),
				}),
			},
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					page := int(calls.Add(1)) - 1
					if page >= len(test.pages) {
						t.Fatal("incomplete pagination made an unbounded request")
					}
					return featureFlagTestResponse(request, http.StatusOK, test.pages[page]), nil
				},
			))

			flags, err := clientUnderTest.ListFeatureFlags(
				context.Background(),
				environmentOne,
				test.archived,
			)
			if test.wantClassification == "" {
				if err != nil || len(flags) != test.wantCount {
					t.Fatal("complete page was not accepted")
				}
				return
			}
			requireAPIErrorClassification(t, err, test.wantClassification)
			if flags != nil {
				t.Fatal("malformed or incomplete pagination returned a partial collection")
			}
		})
	}
}

func TestResolveFeatureFlagByKeyDistinguishesExactStatuses(t *testing.T) {
	t.Parallel()

	activeExact := featureFlagResultForTest(featureFlagIDOne, "exact", false)
	archivedExact := featureFlagResultForTest(featureFlagIDTwo, "exact", true)
	activeDuplicate := featureFlagResultForTest(featureFlagIDThree, "exact", false)
	archivedDuplicate := featureFlagResultForTest(featureFlagIDThree, "exact", true)

	tests := map[string]struct {
		active             []FeatureFlag
		archived           []FeatureFlag
		wantStatus         FeatureFlagStatus
		wantID             string
		wantClassification Classification
	}{
		"exact zero ignores fuzzy and different case": {
			active: []FeatureFlag{
				featureFlagResultForTest(featureFlagIDOne, "exact-extra", false),
				featureFlagResultForTest(featureFlagIDTwo, "Exact", false),
			},
			wantStatus: FeatureFlagStatusAbsent,
		},
		"exact active": {
			active:     []FeatureFlag{activeExact},
			wantStatus: FeatureFlagStatusActive,
			wantID:     featureFlagIDOne,
		},
		"exact archived": {
			archived:   []FeatureFlag{archivedExact},
			wantStatus: FeatureFlagStatusArchived,
			wantID:     featureFlagIDTwo,
		},
		"duplicate active": {
			active:             []FeatureFlag{activeExact, activeDuplicate},
			wantClassification: ClassificationAmbiguous,
		},
		"duplicate archived": {
			archived:           []FeatureFlag{archivedExact, archivedDuplicate},
			wantClassification: ClassificationAmbiguous,
		},
		"same key in both views": {
			active:             []FeatureFlag{activeExact},
			archived:           []FeatureFlag{archivedExact},
			wantClassification: ClassificationAmbiguous,
		},
		"active view contains archived object": {
			active:             []FeatureFlag{archivedExact},
			wantClassification: ClassificationAmbiguous,
		},
		"archived view contains active object": {
			archived:           []FeatureFlag{activeExact},
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			flag, status, err := resolveFeatureFlagByKey(
				test.active,
				test.archived,
				"exact",
				NewRedactor("synthetic"),
			)
			if test.wantClassification != "" {
				requireAPIErrorClassification(t, err, test.wantClassification)
				if status != FeatureFlagStatusUnknown {
					t.Fatal("ambiguous resolution returned a usable status")
				}
				return
			}
			if err != nil || status != test.wantStatus || flag.ID != test.wantID {
				t.Fatal("exact status resolver returned an unexpected outcome")
			}
		})
	}
}

func TestGetFeatureFlagFallbackComposesActiveAndArchivedViews(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		active             []string
		archived           []string
		wantStatus         FeatureFlagStatus
		wantID             string
		wantClassification Classification
	}{
		"exact zero": {
			active:     []string{},
			archived:   []string{},
			wantStatus: FeatureFlagStatusAbsent,
		},
		"active exact ignores fuzzy archived": {
			active: []string{
				featureFlagListItemTestJSON(featureFlagIDOne, "active", "exact"),
			},
			archived: []string{
				featureFlagListItemTestJSON(featureFlagIDTwo, "fuzzy", "exact-extra"),
			},
			wantStatus: FeatureFlagStatusActive,
			wantID:     featureFlagIDOne,
		},
		"archived exact ignores different case": {
			active: []string{
				featureFlagListItemTestJSON(featureFlagIDOne, "case", "Exact"),
			},
			archived: []string{
				featureFlagListItemTestJSON(featureFlagIDTwo, "archived", "exact"),
			},
			wantStatus: FeatureFlagStatusArchived,
			wantID:     featureFlagIDTwo,
		},
		"duplicate active exact keys": {
			active: []string{
				featureFlagListItemTestJSON(featureFlagIDOne, "first", "exact"),
				featureFlagListItemTestJSON(featureFlagIDTwo, "second", "exact"),
			},
			archived:           []string{},
			wantClassification: ClassificationAmbiguous,
		},
		"inconsistent views": {
			active: []string{
				featureFlagListItemTestJSON(featureFlagIDOne, "active", "exact"),
			},
			archived: []string{
				featureFlagListItemTestJSON(featureFlagIDTwo, "archived", "exact"),
			},
			wantClassification: ClassificationAmbiguous,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					call := calls.Add(1)
					if request.URL.RawQuery == "" {
						if call != 1 {
							t.Fatal("exact fallback repeated the direct request")
						}
						return featureFlagTestResponse(request, http.StatusNotFound, "null"), nil
					}
					archived := request.URL.Query().Get("IsArchived") == "true"
					items := test.active
					if archived {
						items = test.archived
					}
					return featureFlagTestResponse(
						request,
						http.StatusOK,
						featureFlagPageTestJSON(int64(len(items)), items),
					), nil
				},
			))

			flag, status, err := clientUnderTest.GetFeatureFlag(
				context.Background(),
				environmentOne,
				"exact",
			)
			if test.wantClassification != "" {
				requireAPIErrorClassification(t, err, test.wantClassification)
				if status != FeatureFlagStatusUnknown {
					t.Fatal("ambiguous fallback returned a usable status")
				}
			} else if err != nil || status != test.wantStatus || flag.ID != test.wantID {
				t.Fatal("complete active/archived fallback returned an unexpected outcome")
			}
			if calls.Load() != 3 {
				t.Fatalf("fallback request count = %d, want direct + two views", calls.Load())
			}
		})
	}
}

func TestFeatureFlagReadValidationAndCancellation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("request must not execute")
		},
	))

	_, _, err := clientUnderTest.GetFeatureFlag(context.Background(), "invalid", "key")
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, _, err = clientUnderTest.GetFeatureFlag(context.Background(), environmentOne, "")
	requireAPIErrorClassification(t, err, ClassificationValidation)
	_, err = clientUnderTest.ListFeatureFlags(context.Background(), "invalid", false)
	requireAPIErrorClassification(t, err, ClassificationValidation)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = clientUnderTest.GetFeatureFlag(ctx, environmentOne, "exact")
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("Feature Flag read cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid or canceled reads executed %d transport calls", calls.Load())
	}
}

func TestListFeatureFlagsCancellationStopsBetweenPages(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			cancel()
			return featureFlagTestResponse(
				request,
				http.StatusOK,
				featureFlagPageTestJSON(2, []string{
					featureFlagListItemTestJSON(featureFlagIDOne, "first", "first"),
				}),
			), nil
		},
	))

	_, err := clientUnderTest.ListFeatureFlags(ctx, environmentOne, false)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("pagination cancellation sentinel was not preserved")
	}
	if calls.Load() != 1 {
		t.Fatalf("pagination cancellation executed %d transport calls, want 1", calls.Load())
	}
}

func TestListFeatureFlagsUsesBodylessGETRetryBoundary(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	var waits atomic.Int32
	options := defaultTestOptions()
	options.MaxRetries = 1
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			if request.Body != nil && request.Body != http.NoBody {
				t.Fatal("Feature Flag collection GET unexpectedly had a body")
			}
			if attempts.Add(1) == 1 {
				return featureFlagTestResponse(
					request,
					http.StatusServiceUnavailable,
					"null",
				), nil
			}
			return featureFlagTestResponse(
				request,
				http.StatusOK,
				featureFlagPageTestJSON(0, []string{}),
			), nil
		},
	))
	clientUnderTest.retries.wait = func(context.Context, time.Duration) error {
		waits.Add(1)
		return nil
	}

	flags, err := clientUnderTest.ListFeatureFlags(context.Background(), environmentOne, false)
	if err != nil || len(flags) != 0 {
		t.Fatal("safe Feature Flag GET did not recover from a retryable response")
	}
	if attempts.Load() != 2 || waits.Load() != 1 {
		t.Fatal("Feature Flag GET used the wrong retry or wait count")
	}
}

func TestResolveFeatureFlagAlwaysConsumesCompleteActiveAndArchivedViews(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			call := calls.Add(1)
			if request.Method != http.MethodGet ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/feature-flags" {
				t.Fatalf("unexpected resolver request %s %s", request.Method, request.URL.EscapedPath())
			}
			wantArchived := "false"
			if call == 2 {
				wantArchived = "true"
			}
			if call > 2 || request.URL.Query().Get("IsArchived") != wantArchived {
				t.Fatalf("resolver view %d query = %q", call, request.URL.RawQuery)
			}
			return featureFlagTestResponse(
				request,
				http.StatusOK,
				featureFlagPageTestJSON(0, []string{}),
			), nil
		},
	))

	_, status, err := clientUnderTest.ResolveFeatureFlag(
		context.Background(),
		environmentOne,
		"exact-key",
	)
	if err != nil || status != FeatureFlagStatusAbsent {
		t.Fatalf("ResolveFeatureFlag() = %q, %v", status, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("resolver request count = %d, want two complete views", calls.Load())
	}
}

func TestCreateFeatureFlagUsesDocumentedOneShotPayload(t *testing.T) {
	t.Parallel()

	input := CreateFeatureFlagRequest{
		Name:          "Create Flag",
		Key:           "create-key",
		IsEnabled:     false,
		Description:   "Create definition",
		VariationType: "number",
		Variations: []FeatureFlagVariation{
			{ID: featureFlagVariationOne, Name: "First", Value: "1"},
			{ID: featureFlagVariationTwo, Name: "Second", Value: "2"},
		},
		EnabledVariationID:  featureFlagVariationOne,
		DisabledVariationID: featureFlagVariationOne,
		Tags:                []string{},
	}

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			if request.Method != http.MethodPost ||
				request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+"/feature-flags" ||
				request.URL.RawQuery != "" {
				t.Fatalf("unexpected create request %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			if request.Header.Get("Content-Type") != "application/json" ||
				request.Header.Get("Authorization") != syntheticAccessToken {
				t.Fatal("create request omitted required JSON or authorization header")
			}
			for _, header := range contextHeaders {
				if request.Header.Get(header) != "" {
					t.Fatalf("create request sent unsupported context header %q", header)
				}
			}
			var payload CreateFeatureFlagRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode create payload: %v", err)
			}
			if payload.Name != input.Name || payload.Key != input.Key || payload.IsEnabled ||
				payload.Description != input.Description || payload.VariationType != input.VariationType ||
				payload.EnabledVariationID != featureFlagVariationOne ||
				payload.DisabledVariationID != featureFlagVariationOne ||
				len(payload.Variations) != 2 || payload.Variations[1].Value != "2" ||
				payload.Tags == nil || len(payload.Tags) != 0 {
				t.Fatal("create payload did not preserve the complete deterministic seed")
			}

			data := `{"id":"` + featureFlagIDOne + `","envId":"` + environmentOne +
				`","name":"Create Flag","description":"Create definition","key":"create-key",` +
				`"variationType":"number","variations":[` +
				`{"id":"` + featureFlagVariationTwo + `","name":"Second","value":"2"},` +
				`{"id":"` + featureFlagVariationOne + `","name":"First","value":"1"}],` +
				`"isArchived":false,"isEnabled":false,"tags":[]}`
			return featureFlagTestResponse(request, http.StatusOK, data), nil
		},
	))

	created, err := clientUnderTest.CreateFeatureFlag(
		context.Background(),
		environmentOne,
		input,
	)
	if err != nil {
		t.Fatalf("CreateFeatureFlag() error = %v", err)
	}
	if created.ID != featureFlagIDOne || created.Key != input.Key ||
		created.EnvironmentID != environmentOne || created.IsArchived ||
		len(created.Variations) != 2 {
		t.Fatal("CreateFeatureFlag() did not return the safe exact response")
	}
	if calls.Load() != 1 {
		t.Fatalf("create request count = %d, want one", calls.Load())
	}
}

func TestCreateFeatureFlagValidationCancellationAndFailureDoNotRetry(t *testing.T) {
	t.Parallel()

	input := CreateFeatureFlagRequest{
		Name:                "One Shot",
		Key:                 "one-shot",
		VariationType:       "string",
		Variations:          []FeatureFlagVariation{{ID: featureFlagVariationOne, Name: "One", Value: "value"}},
		EnabledVariationID:  featureFlagVariationOne,
		DisabledVariationID: featureFlagVariationOne,
		Tags:                []string{},
	}
	var calls atomic.Int32
	options := defaultTestOptions()
	options.MaxRetries = 3
	clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return featureFlagTestResponse(request, http.StatusServiceUnavailable, "null"), nil
		},
	))

	_, err := clientUnderTest.CreateFeatureFlag(context.Background(), "invalid", input)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	invalidKey := input
	invalidKey.Key = ""
	_, err = clientUnderTest.CreateFeatureFlag(context.Background(), environmentOne, invalidKey)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = clientUnderTest.CreateFeatureFlag(ctx, environmentOne, input)
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("create cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid or canceled create reached the transport")
	}

	_, err = clientUnderTest.CreateFeatureFlag(context.Background(), environmentOne, input)
	requireAPIErrorClassification(t, err, ClassificationTransientServer)
	if calls.Load() != 1 {
		t.Fatalf("mutation attempts = %d, want exactly one", calls.Load())
	}
}

func TestFeatureFlagSpecializedMutationContracts(t *testing.T) {
	t.Parallel()

	const (
		key  = "mutation-key"
		name = "Updated Name"
	)
	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(request *http.Request) (*http.Response, error) {
			call := calls.Add(1)
			for _, header := range contextHeaders {
				if request.Header.Get(header) != "" {
					t.Fatalf("specialized mutation sent unsupported context header %q", header)
				}
			}
			switch call {
			case 1:
				if request.Method != http.MethodPut || request.URL.RawQuery != "" ||
					request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+
						"/feature-flags/"+key+"/name" ||
					request.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected name mutation %s %s", request.Method, request.URL.EscapedPath())
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatalf("decode name mutation: %v", err)
				}
				var actualName string
				if len(body) != 1 || json.Unmarshal(body["name"], &actualName) != nil ||
					actualName != name {
					t.Fatal("name mutation body contained more than the owned name field")
				}
				return featureFlagTestResponse(
					request,
					http.StatusOK,
					`"`+featureFlagIDOne+`"`,
				), nil
			case 2:
				if request.Method != http.MethodPut || request.URL.RawQuery != "" ||
					request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+
						"/feature-flags/"+key+"/archive" {
					t.Fatalf("unexpected archive mutation %s %s", request.Method, request.URL.EscapedPath())
				}
				if request.Body != nil && request.Body != http.NoBody ||
					request.Header.Get("Content-Type") != "" {
					t.Fatal("archive mutation unexpectedly sent the optional comment body")
				}
				return featureFlagTestResponse(request, http.StatusOK, "true"), nil
			case 3:
				if request.Method != http.MethodDelete || request.URL.RawQuery != "" ||
					request.URL.EscapedPath() != "/api/v1/envs/"+environmentOne+
						"/feature-flags/"+key {
					t.Fatalf("unexpected permanent delete %s %s", request.Method, request.URL.EscapedPath())
				}
				if request.Body != nil && request.Body != http.NoBody ||
					request.Header.Get("Content-Type") != "" {
					t.Fatal("permanent delete unexpectedly sent the optional comment body")
				}
				return featureFlagTestResponse(request, http.StatusOK, "true"), nil
			default:
				t.Fatal("specialized mutation executed an unexpected request")
				return nil, nil
			}
		},
	))

	if err := clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		environmentOne,
		key,
		featureFlagIDOne,
		UpdateFeatureFlagNameRequest{Name: name},
	); err != nil {
		t.Fatalf("UpdateFeatureFlagName() error = %v", err)
	}
	if err := clientUnderTest.ArchiveFeatureFlag(
		context.Background(),
		environmentOne,
		key,
	); err != nil {
		t.Fatalf("ArchiveFeatureFlag() error = %v", err)
	}
	if err := clientUnderTest.DeleteFeatureFlag(
		context.Background(),
		environmentOne,
		key,
	); err != nil {
		t.Fatalf("DeleteFeatureFlag() error = %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("specialized mutation calls = %d, want three", calls.Load())
	}
}

func TestFeatureFlagMutationsAreOneShotAndValidateResults(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		invoke func(*Client) error
	}{
		"name": {
			invoke: func(apiClient *Client) error {
				return apiClient.UpdateFeatureFlagName(
					context.Background(),
					environmentOne,
					"one-shot",
					featureFlagIDOne,
					UpdateFeatureFlagNameRequest{Name: "One Shot"},
				)
			},
		},
		"archive": {
			invoke: func(apiClient *Client) error {
				return apiClient.ArchiveFeatureFlag(context.Background(), environmentOne, "one-shot")
			},
		},
		"delete": {
			invoke: func(apiClient *Client) error {
				return apiClient.DeleteFeatureFlag(context.Background(), environmentOne, "one-shot")
			},
		},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			options := defaultTestOptions()
			options.MaxRetries = 3
			clientUnderTest := newTestClientWithTransport(t, options, roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					calls.Add(1)
					return featureFlagTestResponse(request, http.StatusServiceUnavailable, "null"), nil
				},
			))
			err := test.invoke(clientUnderTest)
			requireAPIErrorClassification(t, err, ClassificationTransientServer)
			if calls.Load() != 1 {
				t.Fatalf("%s mutation attempts = %d, want one", name, calls.Load())
			}
		})
	}

	t.Run("mismatched name identity", func(t *testing.T) {
		t.Parallel()
		clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
			func(request *http.Request) (*http.Response, error) {
				return featureFlagTestResponse(
					request,
					http.StatusOK,
					`"`+featureFlagIDTwo+`"`,
				), nil
			},
		))
		err := clientUnderTest.UpdateFeatureFlagName(
			context.Background(),
			environmentOne,
			"mismatch",
			featureFlagIDOne,
			UpdateFeatureFlagNameRequest{Name: "Mismatch"},
		)
		requireAPIErrorClassification(t, err, ClassificationAmbiguous)
	})

	for _, operation := range []string{"archive", "delete"} {
		operation := operation
		t.Run(operation+" false result", func(t *testing.T) {
			t.Parallel()
			clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
				func(request *http.Request) (*http.Response, error) {
					return featureFlagTestResponse(request, http.StatusOK, "false"), nil
				},
			))
			var err error
			if operation == "archive" {
				err = clientUnderTest.ArchiveFeatureFlag(context.Background(), environmentOne, "false-result")
			} else {
				err = clientUnderTest.DeleteFeatureFlag(context.Background(), environmentOne, "false-result")
			}
			requireAPIErrorClassification(t, err, ClassificationAmbiguous)
		})
	}
}

func TestFeatureFlagMutationValidationAndCancellation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	clientUnderTest := newTestClientWithTransport(t, defaultTestOptions(), roundTripFunc(
		func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("invalid mutation reached transport")
		},
	))

	err := clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		"invalid",
		"key",
		featureFlagIDOne,
		UpdateFeatureFlagNameRequest{Name: "Name"},
	)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	err = clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		environmentOne,
		"",
		featureFlagIDOne,
		UpdateFeatureFlagNameRequest{Name: "Name"},
	)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	err = clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		environmentOne,
		"key",
		"invalid",
		UpdateFeatureFlagNameRequest{Name: "Name"},
	)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	err = clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		environmentOne,
		"key",
		featureFlagIDOne,
		UpdateFeatureFlagNameRequest{},
	)
	requireAPIErrorClassification(t, err, ClassificationValidation)
	err = clientUnderTest.ArchiveFeatureFlag(context.Background(), "invalid", "key")
	requireAPIErrorClassification(t, err, ClassificationValidation)
	err = clientUnderTest.DeleteFeatureFlag(context.Background(), environmentOne, "")
	requireAPIErrorClassification(t, err, ClassificationValidation)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = clientUnderTest.ArchiveFeatureFlag(ctx, environmentOne, "canceled")
	apiError := requireAPIErrorClassification(t, err, ClassificationCanceled)
	if !errors.Is(apiError, context.Canceled) {
		t.Fatal("mutation cancellation sentinel was not preserved")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid or canceled mutations executed %d transport calls", calls.Load())
	}
}

func TestFeatureFlagMutationFailuresRedactRuntimeValues(t *testing.T) {
	t.Parallel()

	const (
		tokenMarker      = "api-feature-flag-mutation-token-marker"
		keyMarker        = "feature-flag-mutation-key-marker"
		nameMarker       = "feature-flag-mutation-name-marker"
		valueMarker      = "feature-flag-mutation-value-marker"
		tagMarker        = "feature-flag-mutation-tag-marker"
		targetingMarker  = "feature-flag-mutation-targeting-marker"
		ruleMarker       = "feature-flag-mutation-rule-marker"
		tenantMarker     = "featbit:tenant:feature-flag-mutation-marker"
		rawBodyMarker    = "feature-flag-mutation-raw-body-marker"
		serverHostMarker = "feature-flag-mutation-server-marker.example.invalid"
	)
	detail := strings.Join([]string{
		tokenMarker,
		environmentOne,
		featureFlagIDOne,
		keyMarker,
		nameMarker,
		valueMarker,
		tagMarker,
		targetingMarker,
		ruleMarker,
		tenantMarker,
		"/api/v1/envs/" + environmentOne + "/feature-flags/" + keyMarker + "/name",
		rawBodyMarker,
	}, " | ")
	body, err := json.Marshal(map[string]any{
		"success": false,
		"data":    nil,
		"errors":  []string{detail},
	})
	if err != nil {
		t.Fatal("could not construct mutation redaction response")
	}

	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://"+serverHostMarker+"/api/v1"),
		tokenMarker,
		options,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusBadRequest,
				io.NopCloser(strings.NewReader(string(body))),
			), nil
		}),
	)
	if err != nil {
		t.Fatal("could not construct mutation redaction client")
	}

	updateErr := clientUnderTest.UpdateFeatureFlagName(
		context.Background(),
		environmentOne,
		keyMarker,
		featureFlagIDOne,
		UpdateFeatureFlagNameRequest{Name: nameMarker},
	)
	createInput := CreateFeatureFlagRequest{
		Name:          nameMarker,
		Key:           keyMarker,
		Description:   "feature-flag-mutation-description-marker",
		VariationType: "string",
		Variations: []FeatureFlagVariation{
			{ID: featureFlagVariationOne, Name: "Runtime Variation", Value: valueMarker},
		},
		EnabledVariationID:  featureFlagVariationOne,
		DisabledVariationID: featureFlagVariationOne,
		Tags:                []string{tagMarker},
	}
	_, createErr := clientUnderTest.CreateFeatureFlag(
		context.Background(),
		environmentOne,
		createInput,
	)

	for _, mutationErr := range []error{updateErr, createErr} {
		apiError := requireAPIErrorClassification(t, mutationErr, ClassificationValidation)
		formatted := fmt.Sprintf(
			"%v|%+v|%#v|%v|%+v|%#v|%v|%+v|%#v",
			apiError,
			apiError,
			apiError,
			UpdateFeatureFlagNameRequest{Name: nameMarker},
			UpdateFeatureFlagNameRequest{Name: nameMarker},
			UpdateFeatureFlagNameRequest{Name: nameMarker},
			createInput,
			createInput,
			createInput,
		)
		for _, unsafe := range []string{
			tokenMarker,
			environmentOne,
			featureFlagIDOne,
			featureFlagVariationOne,
			keyMarker,
			nameMarker,
			valueMarker,
			tagMarker,
			targetingMarker,
			ruleMarker,
			tenantMarker,
			rawBodyMarker,
			serverHostMarker,
		} {
			if strings.Contains(formatted, unsafe) ||
				strings.Contains(strings.Join(apiError.Details(), " | "), unsafe) {
				t.Fatal("Feature Flag mutation error exposed a runtime identity or value")
			}
		}
	}
}

func TestFeatureFlagReadFailuresRedactRuntimeValues(t *testing.T) {
	t.Parallel()

	const (
		tokenMarker      = "api-feature-flag-read-token-marker"
		keyMarker        = "feature-flag-runtime-key-marker"
		valueMarker      = "feature-flag-runtime-value-marker"
		tagMarker        = "feature-flag-runtime-tag-marker"
		targetingMarker  = "feature-flag-runtime-targeting-marker"
		ruleMarker       = "feature-flag-runtime-rule-marker"
		rolloutMarker    = "feature-flag-runtime-rollout-marker"
		rawBodyMarker    = "feature-flag-raw-body-marker"
		tenantMarker     = "featbit:tenant:feature-flag-marker"
		serverHostMarker = "feature-flag-server-marker.example.invalid"
	)
	pathMarker := "/api/v1/envs/" + environmentOne + "/feature-flags/" + keyMarker
	detail := strings.Join([]string{
		tokenMarker,
		environmentOne,
		featureFlagIDOne,
		featureFlagVariationOne,
		keyMarker,
		valueMarker,
		tagMarker,
		targetingMarker,
		ruleMarker,
		rolloutMarker,
		tenantMarker,
		pathMarker,
		rawBodyMarker,
	}, " | ")
	body, err := json.Marshal(map[string]any{
		"success": false,
		"data":    nil,
		"errors":  []string{detail},
	})
	if err != nil {
		t.Fatal("could not construct Feature Flag redaction response")
	}

	options := defaultTestOptions()
	options.MaxRetries = 0
	clientUnderTest, err := newClient(
		mustParseURL(t, "https://"+serverHostMarker+"/api/v1"),
		tokenMarker,
		options,
		roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return syntheticResponse(
				request,
				http.StatusBadRequest,
				io.NopCloser(strings.NewReader(string(body))),
			), nil
		}),
	)
	if err != nil {
		t.Fatal("could not construct Feature Flag redaction client")
	}

	_, _, readErr := clientUnderTest.GetFeatureFlag(
		context.Background(),
		environmentOne,
		keyMarker,
	)
	apiError := requireAPIErrorClassification(t, readErr, ClassificationValidation)
	formatted := fmt.Sprintf("%v|%+v|%#v|%s", apiError, apiError, apiError, apiError)
	for _, unsafe := range []string{
		tokenMarker,
		environmentOne,
		featureFlagIDOne,
		featureFlagVariationOne,
		keyMarker,
		valueMarker,
		tagMarker,
		targetingMarker,
		ruleMarker,
		rolloutMarker,
		tenantMarker,
		pathMarker,
		rawBodyMarker,
		serverHostMarker,
	} {
		if strings.Contains(formatted, unsafe) {
			t.Fatal("formatted Feature Flag read error exposed a runtime or server value")
		}
	}

	redactedDetails := strings.Join(apiError.Details(), " | ")
	for _, unsafe := range []string{
		tokenMarker,
		environmentOne,
		featureFlagIDOne,
		featureFlagVariationOne,
		keyMarker,
		valueMarker,
		tagMarker,
		targetingMarker,
		ruleMarker,
		rolloutMarker,
		tenantMarker,
		pathMarker,
		rawBodyMarker,
	} {
		if strings.Contains(redactedDetails, unsafe) {
			t.Fatal("Feature Flag APIError details exposed runtime or server data")
		}
	}
}

func featureFlagTestResponse(request *http.Request, status int, data string) *http.Response {
	success := status >= http.StatusOK && status < http.StatusMultipleChoices
	body := `{"success":` + strconv.FormatBool(success) + `,"data":` + data + `,"errors":[]}`
	return syntheticResponse(request, status, io.NopCloser(strings.NewReader(body)))
}

func featureFlagExactTestJSON(id, environmentID, key string, archived bool) string {
	return `{"id":"` + id + `","envId":"` + environmentID +
		`","name":"Flag","description":"Definition","key":"` + key +
		`","variationType":"string","variations":[{"id":"` +
		featureFlagVariationOne + `","name":"First","value":"value"}],` +
		`"isArchived":` + strconv.FormatBool(archived) + `}`
}

func featureFlagListItemTestJSON(id, name, key string) string {
	return `{"id":"` + id + `","name":"` + name +
		`","description":"Definition","key":"` + key +
		`","variationType":"string","variations":[{"id":"` +
		featureFlagVariationOne + `","name":"First","value":"value"}]}`
}

func featureFlagPageTestJSON(totalCount int64, items []string) string {
	return `{"totalCount":` + strconv.FormatInt(totalCount, 10) +
		`,"items":[` + strings.Join(items, ",") + `]}`
}

func featureFlagResultForTest(id, key string, archived bool) FeatureFlag {
	return FeatureFlag{
		ID:            id,
		EnvironmentID: environmentOne,
		Name:          "Flag",
		Key:           key,
		VariationType: "string",
		Variations: []FeatureFlagVariation{
			{ID: featureFlagVariationOne, Name: "First", Value: "value"},
		},
		IsArchived: archived,
	}
}
