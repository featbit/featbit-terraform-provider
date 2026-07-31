package probe

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestClientUsesDirectAuthorizationAndProducesSanitizedObservation(t *testing.T) {
	t.Parallel()

	const token = "synthetic-direct-authorization-token"
	const responseSecret = "synthetic-environment-secret-value"
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotAuthorization = request.Header.Get("Authorization")
		if request.Header.Get("User-Agent") != probeUserAgent {
			t.Errorf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
		  "success":true,
		  "data":{"items":[{"email":"member@example.test","secrets":[{"name":"Server Key","type":"Server","value":"` + responseSecret + `"}]}],"totalCount":1},
		  "errors":[]
		}`))
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, token)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.DoJSON(context.Background(), http.MethodGet, "/api/v1/projects?pageIndex=0&pageSize=10", nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuthorization != token {
		t.Fatalf("Authorization = %q; token was not sent directly", gotAuthorization)
	}
	if result.Observation.PathTemplate != "/api/v1/projects" {
		t.Fatalf("PathTemplate = %q", result.Observation.PathTemplate)
	}
	if result.Observation.DataShape != "page(items=1,total_count_present=true)" {
		t.Fatalf("DataShape = %q", result.Observation.DataShape)
	}
	output, err := MarshalObservation(result.Observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{token, responseSecret, "member@example.test", "Authorization"} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("observation leaked %q: %s", forbidden, output)
		}
	}
}

func TestNegativeAuthClientOmitsOrUsesOnlySyntheticMalformedToken(t *testing.T) {
	t.Parallel()

	var observed []string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		observed = append(observed, request.Header.Get("Authorization"))
		_, _ = response.Write([]byte(`{"success":false,"data":null,"errors":["Forbidden"]}`))
	}))
	defer server.Close()

	base, err := url.Parse(server.URL + "/api/v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []NegativeAuthCase{NegativeAuthMissing, NegativeAuthMalformed} {
		client, err := NewCloudNegativeAuthClient(testCase, time.Second, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		client.baseURL = base
		if _, err := client.DoJSON(context.Background(), http.MethodGet, "/api/v1/projects", nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(observed) != 2 || observed[0] != "" || observed[1] != syntheticMalformedToken {
		t.Fatalf("negative auth headers = %#v", observed)
	}
}

func TestClientHonorsCancellationAndTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	cfg := testConfig(t, server.URL, syntheticToken)

	timeoutClient, err := NewClient(cfg, TokenService, 10*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, timeoutErr := timeoutClient.DoJSON(context.Background(), http.MethodGet, "/api/v1/projects", nil)
	if timeoutErr == nil || Classify(Observation{}, timeoutErr) != ClassificationTimeout {
		t.Fatalf("timeout classification = %s, error = %v", Classify(Observation{}, timeoutErr), timeoutErr)
	}

	cancelClient, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, cancelErr := cancelClient.DoJSON(ctx, http.MethodGet, "/api/v1/projects", nil)
	if !errors.Is(cancelErr, context.Canceled) {
		t.Fatalf("cancellation error = %v", cancelErr)
	}
}

func TestClientRejectsUndocumentedPathShapeAndCleansUpKnownIdentity(t *testing.T) {
	t.Parallel()

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(
			requests,
			request.Method+" "+request.URL.EscapedPath(),
		)
		if request.Method == http.MethodGet {
			total := 0
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				featureFlagPage{
					TotalCount: &total,
					Items:      []featureFlagListItem{},
				},
			)
			return
		}
		writeLifecycleEnvelope(
			t,
			response,
			http.StatusOK,
			true,
			true,
		)
	}))
	defer server.Close()
	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DoJSON(context.Background(), http.MethodGet, "/private/portal", nil); err == nil {
		t.Fatal("client accepted a path outside documented /api/v1")
	}

	entry := InventoryEntry{
		Type: ResourceFlag,
		Identity: ResourceIdentity{
			EnvironmentID: "environment-test-id",
			Key:           "tfp0-flag-key",
		},
	}
	if err := client.DeleteInventoryEntry(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if strings.Join(requests, ",") !=
		"DELETE /api/v1/envs/environment-test-id/feature-flags/tfp0-flag-key,"+
			"GET /api/v1/envs/environment-test-id/feature-flags,"+
			"GET /api/v1/envs/environment-test-id/feature-flags" {
		t.Fatalf("cleanup requests = %v", requests)
	}
}

func TestCleanupArchivesUnarchivedFeatureFlagBeforeDelete(t *testing.T) {
	t.Parallel()

	exists := true
	archived := false
	requests := []string{}
	flagPath := featureFlagPath(createdEnvironmentID, lifecyclePrefix)
	archivePath := featureFlagArchivePath(
		createdEnvironmentID,
		lifecyclePrefix,
	)
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requests = append(
			requests,
			request.Method+" "+request.URL.EscapedPath(),
		)
		switch {
		case request.Method == http.MethodDelete &&
			request.URL.Path == flagPath &&
			exists &&
			!archived:
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusUnprocessableEntity,
				false,
				nil,
				"CannotDeleteUnarchivedFeatureFlag",
			)
		case request.Method == http.MethodPut &&
			request.URL.Path == archivePath:
			var body resourceChangeRequest
			decodeLifecycleRequest(t, request, &body)
			if body.Comment == "" {
				t.Error("archive cleanup comment is empty")
			}
			archived = true
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				true,
			)
		case request.Method == http.MethodDelete &&
			request.URL.Path == flagPath &&
			exists &&
			archived:
			exists = false
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				true,
			)
		case request.Method == http.MethodGet &&
			request.URL.Path ==
				featureFlagCollectionPath(createdEnvironmentID):
			items := []featureFlagListItem{}
			wantArchived :=
				request.URL.Query().Get("IsArchived") == "true"
			if exists && archived == wantArchived {
				items = append(items, featureFlagListItem{
					ID:  createdFeatureFlagID,
					Key: lifecyclePrefix,
				})
			}
			total := len(items)
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				featureFlagPage{
					TotalCount: &total,
					Items:      items,
				},
			)
		default:
			t.Fatalf(
				"unexpected feature-flag cleanup request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Type: ResourceFlag,
		Identity: ResourceIdentity{
			ProjectID:     createdProjectID,
			EnvironmentID: createdEnvironmentID,
			Key:           lifecyclePrefix,
		},
	}
	if err := client.DeleteInventoryEntry(
		context.Background(),
		entry,
	); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("feature flag remains after archive-before-delete cleanup")
	}
	if strings.Join(client.CleanupWorkarounds(), ",") !=
		"feature_flag_archive_before_delete" {
		t.Fatalf(
			"cleanup workarounds = %v",
			client.CleanupWorkarounds(),
		)
	}
	if len(requests) != 5 ||
		requests[0] != "DELETE "+flagPath ||
		requests[1] != "PUT "+archivePath ||
		requests[2] != "DELETE "+flagPath {
		t.Fatalf("cleanup requests = %v", requests)
	}
}

func TestCleanupAcceptsAmbiguousDeleteOnlyAfterExactAbsence(t *testing.T) {
	t.Parallel()

	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		requests = append(
			requests,
			request.Method+" "+request.URL.EscapedPath(),
		)
		if request.Method == http.MethodDelete {
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusInternalServerError,
				false,
				nil,
			)
			return
		}
		total := 0
		writeLifecycleEnvelope(
			t,
			response,
			http.StatusOK,
			true,
			segmentPage{
				TotalCount: &total,
				Items:      []segmentListItem{},
			},
		)
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Type: ResourceSegment,
		Identity: ResourceIdentity{
			ID:            createdSegmentID,
			ProjectID:     createdProjectID,
			EnvironmentID: createdEnvironmentID,
		},
	}
	if err := client.DeleteInventoryEntry(
		context.Background(),
		entry,
	); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 ||
		!strings.HasPrefix(requests[0], "DELETE ") ||
		!strings.HasPrefix(requests[1], "GET ") ||
		!strings.HasPrefix(requests[2], "GET ") {
		t.Fatalf("cleanup requests = %v", requests)
	}
}

func TestCleanupPreservesPendingEntryWhenExactIdentityRemains(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method == http.MethodDelete {
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				true,
			)
			return
		}
		if request.URL.Query().Get("IsArchived") == "true" {
			total := 0
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				segmentPage{
					TotalCount: &total,
					Items:      []segmentListItem{},
				},
			)
			return
		}
		total := 1
		writeLifecycleEnvelope(
			t,
			response,
			http.StatusOK,
			true,
			segmentPage{
				TotalCount: &total,
				Items: []segmentListItem{{
					ID:  createdSegmentID,
					Key: lifecyclePrefix,
				}},
			},
		)
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Type: ResourceSegment,
		Identity: ResourceIdentity{
			ID:            createdSegmentID,
			ProjectID:     createdProjectID,
			EnvironmentID: createdEnvironmentID,
		},
	}
	cleanupErr := client.DeleteInventoryEntry(
		context.Background(),
		entry,
	)
	if cleanupErr == nil ||
		!strings.Contains(cleanupErr.Error(), "exact identity remains") ||
		strings.Contains(cleanupErr.Error(), createdSegmentID) {
		t.Fatalf("cleanup error = %v", cleanupErr)
	}
}

func TestCleanupUsesOwnedParentAbsenceWhenChildCollectionFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch {
		case request.Method == http.MethodDelete:
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusInternalServerError,
				false,
				nil,
			)
		case request.URL.Path == segmentCollectionPath(createdEnvironmentID):
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusForbidden,
				false,
				nil,
				"Forbidden",
			)
		case request.URL.Path == projectCollectionPath:
			writeLifecycleEnvelope(
				t,
				response,
				http.StatusOK,
				true,
				[]projectWire{},
			)
		default:
			t.Fatalf(
				"unexpected cleanup request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	entry := InventoryEntry{
		Type: ResourceSegment,
		Identity: ResourceIdentity{
			ID:            createdSegmentID,
			ProjectID:     createdProjectID,
			EnvironmentID: createdEnvironmentID,
		},
	}
	if err := client.DeleteInventoryEntry(
		context.Background(),
		entry,
	); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupArchivesSegmentBeforeRequiredDelete(t *testing.T) {
	t.Parallel()

	mock, server, _, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.segmentExists = true
	mock.requireSegmentArchiveDelete = true
	mock.segment = segmentWire{
		ID:                    createdSegmentID,
		EnvID:                 createdEnvironmentID,
		Key:                   lifecyclePrefix,
		Type:                  "environment-specific",
		Scopes:                []string{"organization/synthetic"},
		IsEnvironmentSpecific: true,
	}
	entry := InventoryEntry{
		Type: ResourceSegment,
		Identity: ResourceIdentity{
			ID:            createdSegmentID,
			ProjectID:     createdProjectID,
			EnvironmentID: createdEnvironmentID,
		},
	}
	if err := client.DeleteInventoryEntry(
		context.Background(),
		entry,
	); err != nil {
		t.Fatal(err)
	}
	if mock.segmentExists {
		t.Fatal("segment remains after cleanup")
	}
	if !strings.Contains(
		strings.Join(client.CleanupWorkarounds(), ","),
		"segment_archive_before_delete",
	) {
		t.Fatalf("workarounds = %v", client.CleanupWorkarounds())
	}
}

func TestErrorCodesRejectMessageLikeValues(t *testing.T) {
	t.Parallel()

	raw := []byte(`["Forbidden","person@example.test",{"code":"Required:name","message":"secret"}]`)
	codes := extractErrorCodes(raw)
	got := strings.Join(codes, ",")
	if strings.Contains(got, "person@example.test") || strings.Contains(got, "secret") {
		t.Fatalf("error code extraction leaked message data: %q", got)
	}
	if !strings.Contains(got, "Forbidden") || !strings.Contains(got, "Required:name") {
		t.Fatalf("error codes = %q", got)
	}
}

func testConfig(t *testing.T, serverURL, token string) Config {
	t.Helper()
	parsed, err := url.Parse(serverURL + "/api/v1")
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		APIURL:         parsed,
		ServiceToken:   token,
		Target:         TargetSelfHostedMin,
		ResourcePrefix: "tfp0-20260730-a1",
	}
}
