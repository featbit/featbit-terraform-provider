// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	providerSegmentDeleteFuzzyA = "34343434-3434-4434-8434-343434343434"
	providerSegmentDeleteFuzzyB = "45454545-4545-4454-8454-454545454545"
	providerSegmentDeleteFuzzyC = "56565656-5656-4565-8565-565656565656"
	providerSegmentDeleteFuzzyD = "67676767-6767-4676-8676-676767676767"
)

func TestSegmentResourceDeleteActiveArchivesDeletesAndProvesLaterPageZero(t *testing.T) {
	t.Parallel()

	segmentSchema := segmentResourceSchema()
	priorModel := providerSegmentResourceStateModel()
	prior := canonicalSegmentUpdateModel(t, priorModel)
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	expectations := segmentDeleteStatusExpectations(t, prior, client.SegmentStatusActive)
	expectations = append(
		expectations,
		segmentDeleteReferenceExpectation(http.StatusOK, "[]"),
		segmentDeleteMutationExpectation(http.MethodPut, "/archive", http.StatusOK, "true"),
		segmentDeleteMutationExpectation(http.MethodDelete, "", http.StatusOK, "true"),
		segmentDeleteCollectionPageExpectation(false, 0, 2, []string{
			segmentDeleteListItemJSON(t, segmentDeleteFuzzy(prior, providerSegmentDeleteFuzzyA, "active-a"), false),
		}),
		segmentDeleteCollectionPageExpectation(false, 1, 2, []string{
			segmentDeleteListItemJSON(t, segmentDeleteFuzzy(prior, providerSegmentDeleteFuzzyB, "active-b"), false),
		}),
		segmentDeleteCollectionPageExpectation(true, 0, 2, []string{
			segmentDeleteListItemJSON(t, segmentDeleteFuzzy(prior, providerSegmentDeleteFuzzyC, "archived-a"), true),
		}),
		segmentDeleteCollectionPageExpectation(true, 1, 2, []string{
			segmentDeleteListItemJSON(t, segmentDeleteFuzzy(prior, providerSegmentDeleteFuzzyD, "archived-b"), true),
		}),
	)
	script := &segmentHTTPScript{t: t, expectations: expectations}
	apiClient, closeServer := newProjectResourceTestClient(t, script)
	defer closeServer()
	response := frameworkresource.DeleteResponse{State: priorState}
	(&segmentResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
		t.Fatalf("active Segment Delete diagnostics/state = %v/%v", response.Diagnostics, response.State.Raw)
	}
	script.assertComplete(t)
}

func TestSegmentResourceDeleteAlreadyArchivedAndAbsent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		status       client.SegmentStatus
		expectDelete bool
	}{
		"already archived": {status: client.SegmentStatusArchived, expectDelete: true},
		"already absent":   {status: client.SegmentStatusAbsent},
	}
	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segmentSchema := segmentResourceSchema()
			priorModel := providerSegmentResourceStateModel()
			prior := canonicalSegmentUpdateModel(t, priorModel)
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			expectations := segmentDeleteStatusExpectations(t, prior, test.status)
			if test.expectDelete {
				expectations = append(
					expectations,
					segmentDeleteReferenceExpectation(http.StatusOK, "[]"),
					segmentDeleteMutationExpectation(http.MethodDelete, "", http.StatusOK, "true"),
				)
				expectations = append(
					expectations,
					segmentDeleteStatusExpectations(t, prior, client.SegmentStatusAbsent)...,
				)
			}
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&segmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
				t.Fatalf("%s Delete diagnostics/state = %v/%v", name, response.Diagnostics, response.State.Raw)
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceDeleteReferencePreflightFailsClosed(t *testing.T) {
	t.Parallel()

	validOne := []map[string]any{{
		"envId": providerEnvironmentA,
		"id":    providerFeatureFlagID,
		"name":  "Synthetic reference one",
		"key":   "synthetic-reference-one",
	}}
	validMany := append([]map[string]any{}, validOne...)
	validMany = append(validMany, map[string]any{
		"envId": providerEnvironmentB,
		"id":    providerFeatureFlagSecondID,
		"name":  "Synthetic reference two",
		"key":   "synthetic-reference-two",
	})
	tests := map[string]struct {
		status int
		data   string
	}{
		"one exact reference": {
			status: http.StatusOK,
			data:   segmentDeleteJSON(t, validOne),
		},
		"many exact references": {
			status: http.StatusOK,
			data:   segmentDeleteJSON(t, validMany),
		},
		"duplicate reference result": {
			status: http.StatusOK,
			data:   segmentDeleteJSON(t, []map[string]any{validOne[0], validOne[0]}),
		},
		"malformed reference result": {
			status: http.StatusOK,
			data: segmentDeleteJSON(t, []map[string]any{{
				"envId": providerEnvironmentA,
				"id":    providerFeatureFlagID,
			}}),
		},
		"null reference result": {
			status: http.StatusOK,
			data:   "null",
		},
		"failed reference request": {
			status: http.StatusInternalServerError,
			data:   "null",
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segmentSchema := segmentResourceSchema()
			priorModel := providerSegmentResourceStateModel()
			prior := canonicalSegmentUpdateModel(t, priorModel)
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			expectations := segmentDeleteStatusExpectations(t, prior, client.SegmentStatusActive)
			expectations = append(
				expectations,
				segmentDeleteReferenceExpectation(test.status, test.data),
			)
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&segmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) {
				t.Fatalf("%s did not preserve Segment state: %v", name, response.Diagnostics)
			}
			for _, unsafe := range []string{
				providerEnvironmentA,
				providerEnvironmentB,
				providerSegmentID,
				providerFeatureFlagID,
				providerFeatureFlagSecondID,
				"synthetic-reference-one",
				"synthetic-reference-two",
			} {
				if diagnosticsContain(response.Diagnostics, unsafe) {
					t.Fatal("Segment reference diagnostic exposed a runtime identity or key")
				}
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceDeleteReconcilesArchiveWithoutReplay(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		archiveStatus int
		reconciled    client.SegmentStatus
		wantError     bool
		wantRemoved   bool
	}{
		"archive conflict became archived": {
			archiveStatus: http.StatusConflict,
			reconciled:    client.SegmentStatusArchived,
			wantRemoved:   true,
		},
		"ambiguous archive became absent": {
			archiveStatus: http.StatusServiceUnavailable,
			reconciled:    client.SegmentStatusAbsent,
			wantRemoved:   true,
		},
		"ambiguous archive remains active": {
			archiveStatus: http.StatusServiceUnavailable,
			reconciled:    client.SegmentStatusActive,
			wantError:     true,
		},
		"archive reference conflict remains active": {
			archiveStatus: http.StatusConflict,
			reconciled:    client.SegmentStatusActive,
			wantError:     true,
		},
		"archive validation failure is not replayed": {
			archiveStatus: http.StatusBadRequest,
			wantError:     true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segmentSchema := segmentResourceSchema()
			priorModel := providerSegmentResourceStateModel()
			prior := canonicalSegmentUpdateModel(t, priorModel)
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			expectations := segmentDeleteStatusExpectations(t, prior, client.SegmentStatusActive)
			expectations = append(
				expectations,
				segmentDeleteReferenceExpectation(http.StatusOK, "[]"),
				segmentDeleteMutationExpectation(http.MethodPut, "/archive", test.archiveStatus, "null"),
			)
			if test.reconciled != client.SegmentStatusUnknown {
				expectations = append(
					expectations,
					segmentDeleteStatusExpectations(t, prior, test.reconciled)...,
				)
			}
			if test.reconciled == client.SegmentStatusArchived {
				expectations = append(
					expectations,
					segmentDeleteMutationExpectation(http.MethodDelete, "", http.StatusOK, "true"),
				)
				expectations = append(
					expectations,
					segmentDeleteStatusExpectations(t, prior, client.SegmentStatusAbsent)...,
				)
			}
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&segmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("%s diagnostics = %v", name, response.Diagnostics)
			}
			if test.wantRemoved {
				if !response.State.Raw.IsNull() {
					t.Fatal("reconciled archive exact absence did not remove state")
				}
			} else if !response.State.Raw.Equal(priorState.Raw) {
				t.Fatal("unconfirmed archive changed Segment state")
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceDeleteReconcilesPermanentDeleteAndPreservesFailures(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		deleteStatus int
		proof        *client.SegmentStatus
		incomplete   bool
		wantError    bool
		wantRemoved  bool
	}{
		"ambiguous delete proved absent": {
			deleteStatus: http.StatusServiceUnavailable,
			proof:        segmentDeleteStatusPointer(client.SegmentStatusAbsent),
			wantRemoved:  true,
		},
		"unconfirmed not found delete proved absent": {
			deleteStatus: http.StatusNotFound,
			proof:        segmentDeleteStatusPointer(client.SegmentStatusAbsent),
			wantRemoved:  true,
		},
		"ambiguous delete remains archived": {
			deleteStatus: http.StatusServiceUnavailable,
			proof:        segmentDeleteStatusPointer(client.SegmentStatusArchived),
			wantError:    true,
		},
		"delete validation failure is not replayed": {
			deleteStatus: http.StatusBadRequest,
			wantError:    true,
		},
		"successful delete still present": {
			deleteStatus: http.StatusOK,
			proof:        segmentDeleteStatusPointer(client.SegmentStatusArchived),
			wantError:    true,
		},
		"successful delete has incomplete absence proof": {
			deleteStatus: http.StatusOK,
			incomplete:   true,
			wantError:    true,
		},
	}

	for name, test := range tests {
		name := name
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segmentSchema := segmentResourceSchema()
			priorModel := providerSegmentResourceStateModel()
			prior := canonicalSegmentUpdateModel(t, priorModel)
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			expectations := segmentDeleteStatusExpectations(t, prior, client.SegmentStatusArchived)
			expectations = append(
				expectations,
				segmentDeleteReferenceExpectation(http.StatusOK, "[]"),
				segmentDeleteMutationExpectation(
					http.MethodDelete,
					"",
					test.deleteStatus,
					func() string {
						if test.deleteStatus == http.StatusOK {
							return "true"
						}
						return "null"
					}(),
				),
			)
			if test.incomplete {
				expectations = append(
					expectations,
					segmentDeleteCollectionPageExpectation(false, 0, 1, []string{}),
				)
			} else if test.proof != nil {
				expectations = append(
					expectations,
					segmentDeleteStatusExpectations(t, prior, *test.proof)...,
				)
			}
			script := &segmentHTTPScript{t: t, expectations: expectations}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&segmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("%s diagnostics = %v", name, response.Diagnostics)
			}
			if test.wantRemoved {
				if !response.State.Raw.IsNull() {
					t.Fatal("authoritative ambiguous delete absence did not remove state")
				}
			} else if !response.State.Raw.Equal(priorState.Raw) {
				t.Fatal("failed permanent delete changed Segment state")
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceDeleteRejectsUnsafeAndInconsistentStartingStatus(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, canonicalSegment) []segmentHTTPExpectation{
		"shared segment": func(t *testing.T, prior canonicalSegment) []segmentHTTPExpectation {
			shared := prior
			shared.Type = client.SegmentTypeShared
			shared.Scopes = []string{"organization/synthetic-organization"}
			return segmentDeleteStatusExpectations(t, shared, client.SegmentStatusActive)
		},
		"environment scope mismatch": func(t *testing.T, prior canonicalSegment) []segmentHTTPExpectation {
			drifted := prior
			drifted.Scopes = []string{
				"organization/synthetic-organization:project/synthetic-project:env/synthetic-other-environment",
			}
			return segmentDeleteStatusExpectations(t, drifted, client.SegmentStatusActive)
		},
		"UUID matches but key differs": func(t *testing.T, prior canonicalSegment) []segmentHTTPExpectation {
			mismatch := prior
			mismatch.Key = "synthetic-other-key"
			return []segmentHTTPExpectation{
				segmentDeleteCollectionPageExpectation(false, 0, 1, []string{
					segmentDeleteListItemJSON(t, mismatch, false),
				}),
				segmentDeleteCollectionPageExpectation(true, 0, 0, []string{}),
			}
		},
		"key matches but UUID differs": func(t *testing.T, prior canonicalSegment) []segmentHTTPExpectation {
			mismatch := prior
			mismatch.ID = providerSegmentDeleteFuzzyA
			return []segmentHTTPExpectation{
				segmentDeleteCollectionPageExpectation(false, 0, 1, []string{
					segmentDeleteListItemJSON(t, mismatch, false),
				}),
				segmentDeleteCollectionPageExpectation(true, 0, 0, []string{}),
			}
		},
		"incomplete active collection": func(*testing.T, canonicalSegment) []segmentHTTPExpectation {
			return []segmentHTTPExpectation{
				segmentDeleteCollectionPageExpectation(false, 0, 1, []string{}),
			}
		},
		"cross-view duplicate exact identity": func(t *testing.T, prior canonicalSegment) []segmentHTTPExpectation {
			return []segmentHTTPExpectation{
				segmentDeleteCollectionPageExpectation(false, 0, 1, []string{
					segmentDeleteListItemJSON(t, prior, false),
				}),
				segmentDeleteCollectionPageExpectation(true, 0, 1, []string{
					segmentDeleteListItemJSON(t, prior, true),
				}),
			}
		},
	}

	for name, buildExpectations := range tests {
		name := name
		buildExpectations := buildExpectations
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			segmentSchema := segmentResourceSchema()
			priorModel := providerSegmentResourceStateModel()
			prior := canonicalSegmentUpdateModel(t, priorModel)
			priorState := segmentResourceTestState(t, segmentSchema, priorModel)
			script := &segmentHTTPScript{t: t, expectations: buildExpectations(t, prior)}
			apiClient, closeServer := newProjectResourceTestClient(t, script)
			defer closeServer()
			response := frameworkresource.DeleteResponse{State: priorState}
			(&segmentResource{client: apiClient}).Delete(
				context.Background(),
				frameworkresource.DeleteRequest{State: priorState},
				&response,
			)
			if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) {
				t.Fatalf("%s did not fail closed: %v", name, response.Diagnostics)
			}
			script.assertComplete(t)
		})
	}
}

func TestSegmentResourceDeleteRejectsUnsafeStateBeforeTransport(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { requests.Add(1) },
	))
	defer closeServer()
	priorModel := providerSegmentResourceStateModel()
	priorModel.Type = types.StringValue(string(client.SegmentTypeShared))
	priorModel.Scopes = terraformStringSetValue([]string{"organization/synthetic-organization"})
	segmentSchema := segmentResourceSchema()
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	response := frameworkresource.DeleteResponse{State: priorState}
	(&segmentResource{client: apiClient}).Delete(
		context.Background(),
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) ||
		requests.Load() != 0 {
		t.Fatalf("unsafe Delete diagnostics/state/requests = %v/%t/%d", response.Diagnostics, response.State.Raw.Equal(priorState.Raw), requests.Load())
	}
}

func TestSegmentResourceDeleteCancellationWhileWaitingForWriteLock(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	apiClient, closeServer := newProjectResourceTestClient(t, http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { requests.Add(1) },
	))
	defer closeServer()
	priorModel := providerSegmentResourceStateModel()
	prior := canonicalSegmentUpdateModel(t, priorModel)
	segmentSchema := segmentResourceSchema()
	priorState := segmentResourceTestState(t, segmentSchema, priorModel)
	manager := newKeyedLockManager()
	release, err := manager.acquire(
		context.Background(),
		segmentWriteLockKey(prior.EnvironmentID, prior.ID),
	)
	if err != nil {
		t.Fatalf("occupy Segment delete lock: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response := frameworkresource.DeleteResponse{State: priorState}
	(&segmentResource{client: apiClient, locks: manager}).Delete(
		ctx,
		frameworkresource.DeleteRequest{State: priorState},
		&response,
	)
	release()
	if !response.Diagnostics.HasError() || !response.State.Raw.Equal(priorState.Raw) ||
		requests.Load() != 0 {
		t.Fatalf("canceled Delete diagnostics/state/requests = %v/%t/%d", response.Diagnostics, response.State.Raw.Equal(priorState.Raw), requests.Load())
	}
	manager.mu.Lock()
	remaining := len(manager.locks)
	manager.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("canceled Segment Delete retained %d lock entries", remaining)
	}
}

func segmentDeleteStatusExpectations(
	t *testing.T,
	segment canonicalSegment,
	status client.SegmentStatus,
) []segmentHTTPExpectation {
	t.Helper()
	active := []string{}
	archived := []string{}
	switch status {
	case client.SegmentStatusActive:
		active = []string{segmentDeleteListItemJSON(t, segment, false)}
	case client.SegmentStatusArchived:
		archived = []string{segmentDeleteListItemJSON(t, segment, true)}
	case client.SegmentStatusAbsent:
		// Complete empty views.
	default:
		t.Fatalf("unsupported Segment delete fixture status %q", status)
	}
	return []segmentHTTPExpectation{
		segmentDeleteCollectionPageExpectation(false, 0, int64(len(active)), active),
		segmentDeleteCollectionPageExpectation(true, 0, int64(len(archived)), archived),
	}
}

func segmentDeleteCollectionPageExpectation(
	archived bool,
	pageIndex int64,
	total int64,
	items []string,
) segmentHTTPExpectation {
	return segmentHTTPExpectation{
		method: http.MethodGet,
		path:   segmentResourceCollectionPath(),
		query: fmt.Sprintf(
			"IsArchived=%t&Name=&PageIndex=%d&PageSize=100",
			archived,
			pageIndex,
		),
		status: http.StatusOK,
		data: fmt.Sprintf(
			`{"totalCount":%d,"items":[%s]}`,
			total,
			strings.Join(items, ","),
		),
	}
}

func segmentDeleteReferenceExpectation(status int, data string) segmentHTTPExpectation {
	return segmentHTTPExpectation{
		method: http.MethodGet,
		path:   segmentResourceExactPath(providerSegmentID) + "/flag-references",
		status: status,
		data:   data,
	}
}

func segmentDeleteMutationExpectation(
	method string,
	suffix string,
	status int,
	data string,
) segmentHTTPExpectation {
	return segmentHTTPExpectation{
		method: method,
		path:   segmentResourceExactPath(providerSegmentID) + suffix,
		status: status,
		data:   data,
		checkBody: func(t *testing.T, request *http.Request) {
			if request.Header.Get("Content-Type") != "" ||
				(request.Body != nil && request.Body != http.NoBody) {
				t.Fatal("destructive Segment lifecycle sent the optional comment body")
			}
		},
	}
}

func segmentDeleteListItemJSON(
	t *testing.T,
	segment canonicalSegment,
	archived bool,
) string {
	t.Helper()
	payload := map[string]any{
		"id": segment.ID, "envId": segment.EnvironmentID,
		"name": segment.Name, "key": segment.Key,
		"type": string(segment.Type), "scopes": append([]string{}, segment.Scopes...),
		"isArchived":            archived,
		"isEnvironmentSpecific": segment.Type == client.SegmentTypeEnvironmentSpecific,
	}
	return segmentDeleteJSON(t, payload)
}

func segmentDeleteFuzzy(
	prior canonicalSegment,
	id string,
	keySuffix string,
) canonicalSegment {
	fuzzy := prior
	fuzzy.ID = id
	fuzzy.Key += "-" + keySuffix
	return fuzzy
}

func segmentDeleteJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode Segment delete fixture: %v", err)
	}
	return string(encoded)
}

func segmentDeleteStatusPointer(status client.SegmentStatus) *client.SegmentStatus {
	return &status
}
