package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	lifecyclePrefix          = "tfp0-20260731-l1"
	existingProjectID        = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	createdProjectID         = "11111111-1111-1111-1111-111111111111"
	autoEnvironmentOneID     = "22222222-2222-2222-2222-222222222222"
	autoEnvironmentTwoID     = "33333333-3333-3333-3333-333333333333"
	createdEnvironmentID     = "44444444-4444-4444-4444-444444444444"
	duplicateProjectID       = "55555555-5555-5555-5555-555555555555"
	createdFeatureFlagID     = "66666666-6666-6666-6666-666666666666"
	featureFlagRevision1     = "77777777-7777-7777-7777-777777777777"
	featureFlagRevision2     = "88888888-8888-8888-8888-888888888888"
	createdSegmentID         = "99999999-9999-9999-9999-999999999999"
	duplicateSegmentID       = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	existingSharedSegmentID  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	lifecycleServerSecret    = "synthetic-lifecycle-server-secret"
	lifecycleClientSecret    = "synthetic-lifecycle-client-secret"
	syntheticOrganizationKey = "synthetic-private-organization"
)

type lifecycleMock struct {
	t                           *testing.T
	prefix                      string
	collision                   bool
	ambiguousProjectCreate      bool
	ambiguousEnvironmentCreate  bool
	ambiguousFeatureFlagCreate  bool
	ambiguousSegmentCreate      bool
	ambiguousSegmentScopes      bool
	acceptDuplicateProject      bool
	acceptDuplicateSegment      bool
	requireFlagArchiveDelete    bool
	requireSegmentArchiveDelete bool
	failProjectUpdate           bool
	failFeatureFlagNameUpdate   bool
	failSegmentNameUpdate       bool
	projectExists               bool
	duplicateProjectExists      bool
	environmentExists           bool
	featureFlagExists           bool
	segmentExists               bool
	duplicateSegmentExists      bool
	projectName                 string
	environmentName             string
	environmentDescription      string
	featureFlag                 featureFlagWire
	segment                     segmentWire
	duplicateSegment            segmentWire
	segmentFlagReferences       []segmentFlagReference
	requests                    []string
	mutationPaths               []string
}

func TestProjectEnvironmentLifecycleMutatesOnlyCreatedIDsAndCleansUp(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunProjectEnvironmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Project.Created ||
		!report.Project.Updated ||
		!report.Project.KeyPreserved ||
		!report.Project.CanonicalReadsEqual {
		t.Fatalf("project report = %+v", report.Project)
	}
	if report.Project.AutoEnvironmentCount != 2 ||
		len(report.Project.AutoEnvironments) != 2 ||
		report.Project.AutoEnvironments[0].Key != "prod" ||
		report.Project.AutoEnvironments[1].Key != "dev" {
		t.Fatalf("auto environments = %+v", report.Project.AutoEnvironments)
	}
	if !report.Environment.Created ||
		!report.Environment.Updated ||
		!report.Environment.KeyPreserved ||
		!report.Environment.CanonicalReadsEqual {
		t.Fatalf("environment report = %+v", report.Environment)
	}
	if report.Environment.SecretMetadata.Count != 2 ||
		report.Environment.SecretMetadata.ValueFieldPresent != 2 {
		t.Fatalf("secret metadata = %+v", report.Environment.SecretMetadata)
	}
	if !report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent ||
		report.Cleanup.PendingInventory != 0 {
		t.Fatalf("cleanup report = %+v", report.Cleanup)
	}
	if mock.projectExists || mock.environmentExists {
		t.Fatalf(
			"mock resources remain: project=%t environment=%t",
			mock.projectExists,
			mock.environmentExists,
		)
	}

	for _, path := range mock.mutationPaths {
		for _, forbidden := range []string{
			existingProjectID,
			autoEnvironmentOneID,
			autoEnvironmentTwoID,
		} {
			if strings.Contains(path, forbidden) {
				t.Fatalf("mutation %q targeted non-owned ID %q", path, forbidden)
			}
		}
	}

	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 || len(inventory.Entries) != 2 {
		t.Fatalf("inventory = %+v", inventory)
	}

	output, err := MarshalProjectEnvironmentLifecycleReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		syntheticToken,
		existingProjectID,
		createdProjectID,
		autoEnvironmentOneID,
		autoEnvironmentTwoID,
		createdEnvironmentID,
		lifecycleServerSecret,
		lifecycleClientSecret,
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("lifecycle report leaked %q: %s", forbidden, output)
		}
	}
	for _, observation := range report.Observations {
		if strings.Contains(observation.PathTemplate, createdProjectID) ||
			strings.Contains(observation.PathTemplate, createdEnvironmentID) {
			t.Fatalf("observation leaked a concrete ID: %+v", observation)
		}
	}
}

func TestProjectEnvironmentLifecycleReconcilesAmbiguousCreatesByExactKey(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.ambiguousProjectCreate = true
	mock.ambiguousEnvironmentCreate = true

	report, err := RunProjectEnvironmentLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(report.Workarounds, ",")
	for _, expected := range []string{
		"project_create_exact_key_reconciliation",
		"environment_create_exact_key_reconciliation",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("workarounds = %q; missing %q", got, expected)
		}
	}
	if report.Cleanup.PendingInventory != 0 {
		t.Fatalf("cleanup report = %+v", report.Cleanup)
	}
}

func TestProjectEnvironmentCompatibilityLifecycleRecordsSafeOutcomes(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunProjectEnvironmentCompatibilityLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatibility == nil {
		t.Fatal("compatibility summary is missing")
	}
	for name, outcome := range map[string]CompatibilityOutcome{
		"project validation":     report.Compatibility.ProjectValidation,
		"project duplicate":      report.Compatibility.ProjectDuplicate,
		"environment validation": report.Compatibility.EnvironmentValidation,
		"environment duplicate":  report.Compatibility.EnvironmentDuplicate,
	} {
		if outcome.HTTPStatus != http.StatusBadRequest ||
			outcome.Classification != ClassificationValidation ||
			outcome.EnvelopeSuccess == nil ||
			*outcome.EnvelopeSuccess ||
			outcome.ExactMatchCount != map[string]int{
				"project validation":     0,
				"project duplicate":      1,
				"environment validation": 0,
				"environment duplicate":  1,
			}[name] {
			t.Fatalf("%s outcome = %+v", name, outcome)
		}
	}
	if report.Compatibility.ProjectPresentExactMatches != 1 ||
		report.Compatibility.ProjectAbsentExactMatches != 0 ||
		report.Compatibility.EnvironmentPresentExactMatches != 1 ||
		report.Compatibility.EnvironmentAbsentExactMatches != 0 {
		t.Fatalf("exact collection outcomes = %+v", report.Compatibility)
	}
	for name, outcome := range map[string]CompatibilityOutcome{
		"project":     report.Compatibility.ProjectPostDeleteRead,
		"environment": report.Compatibility.EnvironmentPostDeleteRead,
	} {
		if outcome.HTTPStatus != http.StatusNotFound ||
			outcome.Classification != ClassificationNotFoundUnconfirmed ||
			outcome.EnvelopeSuccess == nil ||
			*outcome.EnvelopeSuccess ||
			outcome.ExactMatchCount != 0 {
			t.Fatalf("%s post-delete outcome = %+v", name, outcome)
		}
	}
	workarounds := strings.Join(report.Workarounds, ",")
	for _, expected := range []string{
		"environment_post_delete_exact_parent_fallback",
		"project_post_delete_exact_collection_fallback",
	} {
		if !strings.Contains(workarounds, expected) {
			t.Fatalf("workarounds = %q; missing %q", workarounds, expected)
		}
	}
	if report.Cleanup.PendingInventory != 0 ||
		mock.projectExists ||
		mock.environmentExists {
		t.Fatalf("cleanup report = %+v, mock = %+v", report.Cleanup, mock)
	}

	output, err := MarshalProjectEnvironmentLifecycleReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		syntheticToken,
		existingProjectID,
		createdProjectID,
		createdEnvironmentID,
		lifecycleServerSecret,
		lifecycleClientSecret,
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("compatibility report leaked %q: %s", forbidden, output)
		}
	}
}

func TestCompatibilityLifecycleTracksUnexpectedDuplicateBeforeStopping(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.acceptDuplicateProject = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunProjectEnvironmentCompatibilityLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "cleanup is pending") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), createdProjectID) ||
		strings.Contains(err.Error(), duplicateProjectID) {
		t.Fatalf("error exposed a concrete ID: %v", err)
	}
	if report.Cleanup.PendingInventory != 2 {
		t.Fatalf("cleanup report = %+v", report.Cleanup)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if inventory.Pending() != 2 || len(inventory.Entries) != 2 {
		t.Fatalf("inventory = %+v", inventory)
	}

	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	if len(results) != 2 || inventory.Pending() != 0 {
		t.Fatalf("cleanup results = %+v, inventory = %+v", results, inventory)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if mock.projectExists || mock.duplicateProjectExists {
		t.Fatalf(
			"mock projects remain: primary=%t duplicate=%t",
			mock.projectExists,
			mock.duplicateProjectExists,
		)
	}
}

func TestFeatureFlagLifecycleUsesOwnedParentAndExactFallbacks(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunFeatureFlagLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.FeatureFlag == nil {
		t.Fatal("feature-flag summary is missing")
	}
	flag := report.FeatureFlag
	if !flag.Created ||
		!flag.NameUpdated ||
		!flag.KeyPreserved ||
		!flag.CanonicalReadsEqual ||
		!flag.UnrelatedFieldsPreserved {
		t.Fatalf("feature-flag lifecycle = %+v", flag)
	}
	if flag.VariationType != "boolean" ||
		flag.VariationCount != 2 ||
		!flag.RequestedVariationIDsPreserved ||
		!flag.VariationValuesCanonical ||
		!flag.EnabledVariationReferenced ||
		!flag.DisabledVariationPreserved ||
		!flag.RevisionPresent ||
		!flag.RevisionChangedAfterName {
		t.Fatalf("feature-flag shape = %+v", flag)
	}
	if flag.Duplicate.HTTPStatus != http.StatusUnprocessableEntity ||
		flag.Duplicate.Classification != ClassificationValidation ||
		flag.Duplicate.ExactMatchCount != 1 ||
		flag.Duplicate.EnvelopeSuccess == nil ||
		*flag.Duplicate.EnvelopeSuccess {
		t.Fatalf("duplicate outcome = %+v", flag.Duplicate)
	}
	if !flag.StaleRevisionAttempted ||
		flag.StaleRevision == nil ||
		flag.StaleRevision.HTTPStatus != http.StatusConflict ||
		flag.StaleRevision.Classification != ClassificationConflict ||
		flag.StaleRevision.EnvelopeSuccess == nil ||
		*flag.StaleRevision.EnvelopeSuccess {
		t.Fatalf("stale-revision outcome = %+v", flag.StaleRevision)
	}
	if flag.PresentExactMatches != 1 ||
		flag.AbsentExactMatches != 0 ||
		flag.PostDeleteRead.HTTPStatus != http.StatusNotFound ||
		flag.PostDeleteRead.Classification !=
			ClassificationNotFoundUnconfirmed {
		t.Fatalf("feature-flag existence = %+v", flag)
	}
	if !report.Cleanup.FeatureFlagAbsent ||
		!report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent ||
		report.Cleanup.PendingInventory != 0 ||
		mock.featureFlagExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}

	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 || len(inventory.Entries) != 3 {
		t.Fatalf("inventory = %+v", inventory)
	}
	workarounds := strings.Join(report.Workarounds, ",")
	for _, expected := range []string{
		"feature_flag_stale_revision_same_value_probe",
		"feature_flag_post_delete_all_page_exact_key_fallback",
	} {
		if !strings.Contains(workarounds, expected) {
			t.Fatalf("workarounds = %q; missing %q", workarounds, expected)
		}
	}

	names := featureFlagNamesForPrefix(lifecyclePrefix)
	output, err := MarshalProjectEnvironmentLifecycleReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		syntheticToken,
		existingProjectID,
		createdProjectID,
		createdEnvironmentID,
		createdFeatureFlagID,
		featureFlagRevision1,
		featureFlagRevision2,
		names.variationOneID,
		names.variationTwoID,
		lifecycleServerSecret,
		lifecycleClientSecret,
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("feature-flag report leaked %q: %s", forbidden, output)
		}
	}
	for _, path := range mock.mutationPaths {
		for _, forbidden := range []string{
			existingProjectID,
			autoEnvironmentOneID,
			autoEnvironmentTwoID,
		} {
			if strings.Contains(path, forbidden) {
				t.Fatalf("mutation %q targeted non-owned ID %q", path, forbidden)
			}
		}
	}
}

func TestFeatureFlagTypeMatrixLifecycleUsesOneOwnedParentAndThreeTypes(
	t *testing.T,
) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.requireFlagArchiveDelete = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunFeatureFlagTypeMatrixLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatalf("RunFeatureFlagTypeMatrixLifecycle() error = %v", err)
	}
	if report.FeatureFlag != nil {
		t.Fatalf("single feature-flag summary unexpectedly set: %+v", report.FeatureFlag)
	}
	expectedTypes := []string{"string", "number", "json"}
	if len(report.FeatureFlags) != len(expectedTypes) {
		t.Fatalf("feature-flag type summaries = %+v", report.FeatureFlags)
	}
	for index, expectedType := range expectedTypes {
		flag := report.FeatureFlags[index]
		if flag.VariationType != expectedType ||
			!flag.Created ||
			!flag.NameUpdated ||
			!flag.KeyPreserved ||
			!flag.CanonicalReadsEqual ||
			!flag.UnrelatedFieldsPreserved ||
			!flag.RequestedVariationIDsPreserved ||
			!flag.VariationValuesCanonical ||
			!flag.EnabledVariationReferenced ||
			!flag.DisabledVariationPreserved ||
			!flag.RevisionPresent ||
			!flag.RevisionChangedAfterName ||
			!flag.ArchivedBeforeDelete ||
			flag.AbsentExactMatches != 0 {
			t.Fatalf("feature-flag type %q summary = %+v", expectedType, flag)
		}
	}

	createCount := 0
	nameUpdateCount := 0
	variationUpdateCount := 0
	for _, observation := range report.Observations {
		switch {
		case observation.Method == http.MethodPost &&
			observation.PathTemplate == featureFlagCollectionTemplate:
			createCount++
		case observation.Method == http.MethodPut &&
			observation.PathTemplate == featureFlagNameTemplate:
			nameUpdateCount++
		case observation.Method == http.MethodPut &&
			observation.PathTemplate == featureFlagVariationsTemplate:
			variationUpdateCount++
		}
	}
	if createCount != 3 || nameUpdateCount != 3 || variationUpdateCount != 0 {
		t.Fatalf(
			"type-matrix writes create=%d name_update=%d variation_update=%d",
			createCount,
			nameUpdateCount,
			variationUpdateCount,
		)
	}
	if report.Cleanup.PendingInventory != 0 ||
		!report.Cleanup.FeatureFlagAbsent ||
		!report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent ||
		mock.featureFlagExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("type-matrix cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}
	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 || len(inventory.Entries) != 5 {
		t.Fatalf("type-matrix inventory = %+v", inventory)
	}
}

func TestFeatureFlagCRUDLifecycleUsesOneCreateAndOneNormalUpdate(t *testing.T) {
	t.Parallel()

	_, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunFeatureFlagCRUDLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatalf("RunFeatureFlagCRUDLifecycle() error = %v", err)
	}
	if report.FeatureFlag == nil {
		t.Fatal("feature-flag summary is missing")
	}
	if !report.FeatureFlag.Created ||
		!report.FeatureFlag.NameUpdated ||
		!report.FeatureFlag.CanonicalReadsEqual ||
		!report.FeatureFlag.UnrelatedFieldsPreserved ||
		report.FeatureFlag.StaleRevisionAttempted {
		t.Fatalf("feature-flag CRUD summary = %+v", report.FeatureFlag)
	}

	createCount := 0
	nameUpdateCount := 0
	variationUpdateCount := 0
	for _, observation := range report.Observations {
		switch {
		case observation.Method == http.MethodPost &&
			observation.PathTemplate == featureFlagCollectionTemplate:
			createCount++
		case observation.Method == http.MethodPut &&
			observation.PathTemplate == featureFlagNameTemplate:
			nameUpdateCount++
		case observation.Method == http.MethodPut &&
			observation.PathTemplate == featureFlagVariationsTemplate:
			variationUpdateCount++
		}
	}
	if createCount != 1 || nameUpdateCount != 1 || variationUpdateCount != 0 {
		t.Fatalf(
			"flag write counts create=%d name_update=%d variation_update=%d",
			createCount,
			nameUpdateCount,
			variationUpdateCount,
		)
	}
	if report.Cleanup.PendingInventory != 0 ||
		!report.Cleanup.FeatureFlagAbsent ||
		!report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent {
		t.Fatalf("cleanup = %+v", report.Cleanup)
	}
}

func TestFeatureFlagCRUDLifecycleArchivesWhenCloudRejectsDirectDelete(
	t *testing.T,
) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.requireFlagArchiveDelete = true

	report, err := RunFeatureFlagCRUDLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatalf("RunFeatureFlagCRUDLifecycle() error = %v", err)
	}
	flag := report.FeatureFlag
	if flag == nil ||
		!flag.ArchivedBeforeDelete ||
		flag.InitialDelete.HTTPStatus != http.StatusUnprocessableEntity ||
		!observationHasErrorCode(
			Observation{ErrorCodes: flag.InitialDelete.ErrorCodes},
			"CannotDeleteUnarchivedFeatureFlag",
		) ||
		flag.Archive == nil ||
		flag.Archive.Classification != ClassificationSuccess ||
		flag.AbsentExactMatches != 0 {
		t.Fatalf("archive-before-delete summary = %+v", flag)
	}
	if !strings.Contains(
		strings.Join(report.Workarounds, ","),
		"feature_flag_archive_before_delete",
	) {
		t.Fatalf("workarounds = %v", report.Workarounds)
	}
}

func TestFeatureFlagLifecycleReconcilesAmbiguousCreateByAllPageExactKey(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.ambiguousFeatureFlagCreate = true

	report, err := RunFeatureFlagLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		strings.Join(report.Workarounds, ","),
		"feature_flag_create_all_page_exact_key_reconciliation",
	) {
		t.Fatalf("workarounds = %v", report.Workarounds)
	}
	if report.Cleanup.PendingInventory != 0 ||
		mock.featureFlagExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}
}

func TestFeatureFlagLifecycleFailureLeavesParentCascadeRecovery(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.failFeatureFlagNameUpdate = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunFeatureFlagLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("error = %v", err)
	}
	if report.Cleanup.PendingInventory != 3 {
		t.Fatalf("cleanup = %+v", report.Cleanup)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if inventory.Pending() != 3 || len(inventory.Entries) != 3 {
		t.Fatalf("inventory = %+v", inventory)
	}
	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	if len(results) != 3 || inventory.Pending() != 0 {
		t.Fatalf("cleanup results = %+v, inventory = %+v", results, inventory)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if mock.featureFlagExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("mock resources remain: %+v", mock)
	}
}

func TestFeatureFlagExactLookupTraversesAllPagesAndIgnoresFuzzyResults(
	t *testing.T,
) {
	t.Parallel()

	requestedPages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != syntheticToken {
			t.Error("feature-flag exact lookup omitted synthetic authorization")
		}
		pageIndex := request.URL.Query().Get("PageIndex")
		requestedPages = append(requestedPages, pageIndex)
		total := 3
		var items []featureFlagListItem
		switch pageIndex {
		case "0":
			items = []featureFlagListItem{
				{ID: "fuzzy-1", Key: lifecyclePrefix + "-suffix"},
				{ID: "fuzzy-2", Key: "prefix-" + lifecyclePrefix},
			}
		case "1":
			items = []featureFlagListItem{
				{ID: createdFeatureFlagID, Key: lifecyclePrefix},
			}
		default:
			t.Fatalf("unexpected page index %q", pageIndex)
		}
		writeLifecycleEnvelope(
			t,
			response,
			http.StatusOK,
			true,
			featureFlagPage{TotalCount: &total, Items: items},
		)
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	report := ProjectEnvironmentLifecycleReport{}
	match, err := lifecycleFindFeatureFlagByKey(
		context.Background(),
		client,
		&report,
		createdEnvironmentID,
		lifecyclePrefix,
	)
	if err != nil {
		t.Fatal(err)
	}
	if match.Count != 1 ||
		match.Item.ID != createdFeatureFlagID ||
		strings.Join(requestedPages, ",") != "0,1" {
		t.Fatalf("match = %+v, pages = %v", match, requestedPages)
	}
	for _, observation := range report.Observations {
		if observation.PathTemplate != featureFlagCollectionTemplate ||
			strings.Contains(observation.PathTemplate, createdEnvironmentID) ||
			strings.Contains(observation.PathTemplate, lifecyclePrefix) {
			t.Fatalf("unsafe feature-flag observation = %+v", observation)
		}
	}
}

func TestSegmentLifecycleUsesOwnedParentAndExactFallbacks(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunSegmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Segment == nil {
		t.Fatal("segment summary is missing")
	}
	segment := report.Segment
	if !segment.Created ||
		!segment.NameUpdated ||
		!segment.DescriptionUpdated ||
		!segment.TargetingUpdated ||
		!segment.TagsUpdated ||
		!segment.Archived ||
		!segment.Restored ||
		!segment.KeyPreserved ||
		!segment.TypePreserved ||
		!segment.ScopesPreserved ||
		!segment.EnvironmentSpecific ||
		!segment.CanonicalReadsEqual ||
		!segment.UnrelatedFieldsPreserved {
		t.Fatalf("segment lifecycle = %+v", segment)
	}
	if segment.IncludedCount != 2 ||
		segment.ExcludedCount != 1 ||
		segment.RuleCount != 1 ||
		segment.ConditionCount != 1 ||
		segment.TagCount != 2 ||
		segment.FlagReferenceCount != 0 {
		t.Fatalf("segment shape = %+v", segment)
	}
	if segment.Duplicate.HTTPStatus != http.StatusUnprocessableEntity ||
		segment.Duplicate.Classification != ClassificationValidation ||
		segment.Duplicate.ExactMatchCount != 1 ||
		segment.Duplicate.EnvelopeSuccess == nil ||
		*segment.Duplicate.EnvelopeSuccess {
		t.Fatalf("segment duplicate = %+v", segment.Duplicate)
	}
	if segment.PresentExactKeyMatches != 1 ||
		segment.PresentExactIDMatches != 1 ||
		segment.AbsentExactIDMatches != 0 ||
		segment.AbsentExactKeyMatches != 0 ||
		segment.PostDeleteRead.HTTPStatus != http.StatusNotFound ||
		segment.PostDeleteRead.Classification !=
			ClassificationNotFoundUnconfirmed {
		t.Fatalf("segment existence = %+v", segment)
	}
	if !report.Cleanup.SegmentAbsent ||
		!report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent ||
		report.Cleanup.PendingInventory != 0 ||
		mock.segmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}

	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 || len(inventory.Entries) != 3 {
		t.Fatalf("inventory = %+v", inventory)
	}
	workarounds := strings.Join(report.Workarounds, ",")
	for _, expected := range []string{
		"segment_condition_operator_documentation_mapping",
		"segment_delete_flag_reference_preflight",
		"segment_post_delete_all_page_exact_id_fallback",
	} {
		if !strings.Contains(workarounds, expected) {
			t.Fatalf("workarounds = %q; missing %q", workarounds, expected)
		}
	}

	names := segmentNamesForPrefix(lifecyclePrefix)
	output, err := MarshalProjectEnvironmentLifecycleReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		syntheticToken,
		existingProjectID,
		createdProjectID,
		createdEnvironmentID,
		createdSegmentID,
		existingSharedSegmentID,
		syntheticOrganizationKey,
		names.ruleID,
		names.conditionID,
		names.conditionValue,
		names.includedUsers[0],
		names.includedUsers[1],
		names.excludedUsers[0],
		"synthetic-workspace-id",
		lifecycleServerSecret,
		lifecycleClientSecret,
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("segment report leaked %q: %s", forbidden, output)
		}
	}
	for _, path := range mock.mutationPaths {
		for _, forbidden := range []string{
			existingProjectID,
			autoEnvironmentOneID,
			autoEnvironmentTwoID,
		} {
			if strings.Contains(path, forbidden) {
				t.Fatalf("mutation %q targeted non-owned ID %q", path, forbidden)
			}
		}
	}
	for _, request := range mock.requests {
		if strings.HasPrefix(request, http.MethodPatch+" ") {
			t.Fatalf("segment lifecycle used schema-less generic PATCH: %q", request)
		}
	}
}

func TestSegmentCRUDLifecycleCreatesExactlyOneSegment(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()

	report, err := RunSegmentCRUDLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Segment == nil ||
		!report.Segment.Created ||
		!report.Segment.NameUpdated ||
		!report.Segment.DescriptionUpdated ||
		!report.Segment.TargetingUpdated ||
		!report.Segment.TagsUpdated ||
		!report.Segment.Archived ||
		!report.Segment.Restored {
		t.Fatalf("segment lifecycle = %+v", report.Segment)
	}
	if report.Segment.Duplicate.HTTPStatus != 0 ||
		report.Segment.Duplicate.EnvelopeSuccess != nil ||
		report.Segment.Duplicate.Classification != "" {
		t.Fatalf("duplicate check unexpectedly ran: %+v", report.Segment.Duplicate)
	}
	segmentCreates := 0
	expectedCreate := http.MethodPost + " " +
		segmentCollectionPath(createdEnvironmentID)
	for _, request := range mock.requests {
		if request == expectedCreate {
			segmentCreates++
		}
	}
	if segmentCreates != 1 {
		t.Fatalf("segment create count = %d; requests = %v", segmentCreates, mock.requests)
	}
	if report.Cleanup.PendingInventory != 0 ||
		mock.segmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}
}

func TestSegmentCRUDLifecycleArchivesBeforeRequiredDelete(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.requireSegmentArchiveDelete = true

	report, err := RunSegmentCRUDLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	segment := report.Segment
	if segment == nil ||
		!segment.ArchivedBeforeDelete ||
		segment.InitialDelete.HTTPStatus != http.StatusUnprocessableEntity ||
		!observationHasErrorCode(
			Observation{ErrorCodes: segment.InitialDelete.ErrorCodes},
			"CannotDeleteUnArchivedSegment",
		) ||
		segment.DeleteArchive == nil ||
		segment.DeleteArchive.Classification != ClassificationSuccess ||
		segment.AbsentExactIDMatches != 0 ||
		segment.AbsentExactKeyMatches != 0 {
		t.Fatalf("archive-before-delete summary = %+v", segment)
	}
	if !strings.Contains(
		strings.Join(report.Workarounds, ","),
		"segment_archive_before_delete",
	) {
		t.Fatalf("workarounds = %v", report.Workarounds)
	}
}

func TestSegmentCRUDLifecycleRejectsAmbiguousScopeBeforeCreate(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.ambiguousSegmentScopes = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunSegmentCRUDLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "unambiguous") {
		t.Fatalf("error = %v", err)
	}
	if mock.segmentExists {
		t.Fatal("segment was created despite ambiguous scope prefixes")
	}
	expectedCreate := http.MethodPost + " " +
		segmentCollectionPath(createdEnvironmentID)
	for _, request := range mock.requests {
		if request == expectedCreate {
			t.Fatalf("unexpected segment Create: %v", mock.requests)
		}
	}
	if report.Cleanup.PendingInventory != 2 {
		t.Fatalf("cleanup = %+v", report.Cleanup)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if inventory.Pending() != 0 {
		t.Fatalf("cleanup inventory = %+v", inventory)
	}
}

func TestSegmentLifecycleReconcilesAmbiguousCreateByAllPageExactKey(
	t *testing.T,
) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.ambiguousSegmentCreate = true

	report, err := RunSegmentLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		strings.Join(report.Workarounds, ","),
		"segment_create_all_page_exact_key_reconciliation",
	) {
		t.Fatalf("workarounds = %v", report.Workarounds)
	}
	if report.Cleanup.PendingInventory != 0 ||
		mock.segmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}
}

func TestSegmentLifecycleFailureLeavesParentCascadeRecovery(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.failSegmentNameUpdate = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunSegmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("error = %v", err)
	}
	if report.Cleanup.PendingInventory != 3 {
		t.Fatalf("cleanup = %+v", report.Cleanup)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if inventory.Pending() != 3 || len(inventory.Entries) != 3 {
		t.Fatalf("inventory = %+v", inventory)
	}
	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	if len(results) != 3 || inventory.Pending() != 0 {
		t.Fatalf("cleanup results = %+v, inventory = %+v", results, inventory)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if mock.segmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("mock resources remain: %+v", mock)
	}
}

func TestSegmentUnexpectedDuplicateTracksEveryOwnedIdentity(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.acceptDuplicateSegment = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunSegmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "cleanup is pending") {
		t.Fatalf("error = %v", err)
	}
	if report.Cleanup.PendingInventory != 4 {
		t.Fatalf("cleanup = %+v", report.Cleanup)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if inventory.Pending() != 4 || len(inventory.Entries) != 4 {
		t.Fatalf("inventory = %+v", inventory)
	}
	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	if len(results) != 4 || inventory.Pending() != 0 {
		t.Fatalf("cleanup results = %+v, inventory = %+v", results, inventory)
	}
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if mock.segmentExists ||
		mock.duplicateSegmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("mock resources remain: %+v", mock)
	}
}

func TestSegmentReferencePreflightRefusesDelete(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.segmentFlagReferences = []segmentFlagReference{{
		ID:  createdFeatureFlagID,
		Key: lifecyclePrefix + "-flag",
	}}
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunSegmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "references") {
		t.Fatalf("error = %v", err)
	}
	if report.Segment == nil ||
		report.Segment.FlagReferenceCount != 1 ||
		report.Cleanup.PendingInventory != 3 {
		t.Fatalf("report = %+v", report)
	}
	segmentDelete := "DELETE " +
		segmentPath(createdEnvironmentID, createdSegmentID)
	if strings.Contains(strings.Join(mock.mutationPaths, "\n"), segmentDelete) {
		t.Fatalf("referenced segment was deleted: %v", mock.mutationPaths)
	}
	inventory, loadErr := LoadInventory(inventoryPath)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("cleanup result = %+v", result)
		}
	}
	if inventory.Pending() != 0 {
		t.Fatalf("cleanup inventory = %+v", inventory)
	}
}

func TestSegmentExactLookupsTraverseAllPagesAndIgnoreFuzzyResults(
	t *testing.T,
) {
	t.Parallel()

	requestedPages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != syntheticToken {
			t.Error("segment exact lookup omitted synthetic authorization")
		}
		pageIndex := request.URL.Query().Get("PageIndex")
		requestedPages = append(requestedPages, pageIndex)
		total := 3
		var items []segmentListItem
		switch pageIndex {
		case "0":
			items = []segmentListItem{
				{
					ID:  createdSegmentID + "-suffix",
					Key: lifecyclePrefix + "-suffix",
				},
				{
					ID:  "prefix-" + createdSegmentID,
					Key: "prefix-" + lifecyclePrefix,
				},
			}
		case "1":
			items = []segmentListItem{{
				ID:  createdSegmentID,
				Key: lifecyclePrefix,
			}}
		default:
			t.Fatalf("unexpected page index %q", pageIndex)
		}
		writeLifecycleEnvelope(
			t,
			response,
			http.StatusOK,
			true,
			segmentPage{TotalCount: &total, Items: items},
		)
	}))
	defer server.Close()

	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	report := ProjectEnvironmentLifecycleReport{}
	byKey, err := lifecycleFindSegmentByKey(
		context.Background(),
		client,
		&report,
		createdEnvironmentID,
		lifecyclePrefix,
	)
	if err != nil {
		t.Fatal(err)
	}
	byID, err := lifecycleFindSegmentByID(
		context.Background(),
		client,
		&report,
		createdEnvironmentID,
		createdSegmentID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if byKey.Count != 1 ||
		byKey.Item.ID != createdSegmentID ||
		byID.Count != 1 ||
		byID.Item.Key != lifecyclePrefix ||
		strings.Join(requestedPages, ",") != "0,1,0,1" {
		t.Fatalf(
			"key = %+v, id = %+v, pages = %v",
			byKey,
			byID,
			requestedPages,
		)
	}
	for _, observation := range report.Observations {
		if observation.PathTemplate != segmentCollectionTemplate ||
			strings.Contains(observation.PathTemplate, createdEnvironmentID) ||
			strings.Contains(observation.PathTemplate, lifecyclePrefix) ||
			strings.Contains(observation.PathTemplate, createdSegmentID) {
			t.Fatalf("unsafe segment observation = %+v", observation)
		}
	}
}

func TestChildCollectionReadLifecycleMakesNoChildMutationsAndCleansParents(
	t *testing.T,
) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunChildCollectionReadLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.ChildReads == nil {
		t.Fatal("child-read summary is missing")
	}
	childReads := report.ChildReads
	if childReads.FeatureFlagDirectMissing.HTTPStatus !=
		http.StatusNotFound ||
		childReads.FeatureFlagDirectMissing.Classification !=
			ClassificationNotFoundUnconfirmed ||
		childReads.FeatureFlagAbsentExactKeyCount != 0 ||
		childReads.SegmentDirectMissing.HTTPStatus != http.StatusNotFound ||
		childReads.SegmentDirectMissing.Classification !=
			ClassificationNotFoundUnconfirmed ||
		childReads.SegmentAbsentExactIDCount != 0 ||
		childReads.SegmentAbsentExactKeyCount != 0 {
		t.Fatalf("child reads = %+v", childReads)
	}
	if report.FeatureFlag != nil || report.Segment != nil {
		t.Fatalf("read-only report exposed lifecycle summaries: %+v", report)
	}
	if !report.Cleanup.EnvironmentAbsent ||
		!report.Cleanup.ProjectAbsent ||
		report.Cleanup.PendingInventory != 0 ||
		mock.featureFlagExists ||
		mock.segmentExists ||
		mock.environmentExists ||
		mock.projectExists {
		t.Fatalf("cleanup = %+v, mock = %+v", report.Cleanup, mock)
	}
	for _, request := range mock.requests {
		if strings.Contains(request, "/feature-flags") ||
			strings.Contains(request, "/segments") {
			if !strings.HasPrefix(request, http.MethodGet+" ") {
				t.Fatalf("child collection mutation = %q", request)
			}
		}
	}
	for _, path := range mock.mutationPaths {
		if strings.Contains(path, "/feature-flags") ||
			strings.Contains(path, "/segments") {
			t.Fatalf("child mutation path = %q", path)
		}
	}
	workarounds := strings.Join(report.Workarounds, ",")
	for _, expected := range []string{
		"feature_flag_direct_missing_all_page_exact_key_fallback",
		"segment_direct_missing_all_page_exact_id_fallback",
	} {
		if !strings.Contains(workarounds, expected) {
			t.Fatalf("workarounds = %q; missing %q", workarounds, expected)
		}
	}
	output, err := MarshalProjectEnvironmentLifecycleReport(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		syntheticToken,
		createdProjectID,
		createdEnvironmentID,
		deterministicProbeUUID(lifecyclePrefix + ":segment:missing"),
		lifecycleServerSecret,
		lifecycleClientSecret,
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("child-read report leaked %q: %s", forbidden, output)
		}
	}
	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 || len(inventory.Entries) != 2 {
		t.Fatalf("inventory = %+v", inventory)
	}
}

func TestProjectEnvironmentLifecycleRefusesExactPreexistingKey(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.collision = true

	report, err := RunProjectEnvironmentLifecycle(
		context.Background(),
		cfg,
		client,
		filepath.Join(t.TempDir(), "cleanup.json"),
	)
	if err == nil || !strings.Contains(err.Error(), "pre-existing") {
		t.Fatalf("error = %v", err)
	}
	if len(mock.mutationPaths) != 0 {
		t.Fatalf("preflight collision made mutations: %v", mock.mutationPaths)
	}
	if report.Project.Created || report.Cleanup.PendingInventory != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestProjectEnvironmentLifecycleFailureLeavesExactRecoverableInventory(t *testing.T) {
	t.Parallel()

	mock, server, cfg, client := newLifecycleTestServer(t)
	defer server.Close()
	mock.failProjectUpdate = true
	inventoryPath := filepath.Join(t.TempDir(), "cleanup.json")

	report, err := RunProjectEnvironmentLifecycle(
		context.Background(),
		cfg,
		client,
		inventoryPath,
	)
	if err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatalf("error = %v", err)
	}
	if report.Cleanup.PendingInventory != 1 {
		t.Fatalf("cleanup report = %+v", report.Cleanup)
	}
	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 1 ||
		inventory.Entries[0].Identity.ID != createdProjectID ||
		inventory.Entries[0].Identity.Key != lifecyclePrefix {
		t.Fatalf("recoverable inventory = %+v", inventory)
	}

	results := inventory.Cleanup(
		context.Background(),
		false,
		client.DeleteInventoryEntry,
	)
	if len(results) != 1 || results[0].Err != nil || inventory.Pending() != 0 {
		t.Fatalf("cleanup results = %+v, inventory = %+v", results, inventory)
	}
	for _, path := range mock.mutationPaths {
		if strings.Contains(path, existingProjectID) {
			t.Fatalf("recovery targeted pre-existing project: %q", path)
		}
	}
}

func TestClientObservationTemplateAndTransportErrorDoNotExposeConcretePath(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		writeLifecycleEnvelope(t, response, http.StatusOK, true, map[string]interface{}{})
	}))
	cfg := testConfig(t, server.URL, syntheticToken)
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.DoJSONAt(
		context.Background(),
		http.MethodGet,
		projectPath(createdProjectID),
		projectItemTemplate,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Observation.PathTemplate != projectItemTemplate {
		t.Fatalf("PathTemplate = %q", result.Observation.PathTemplate)
	}
	server.Close()
	_, err = client.DoJSONAt(
		context.Background(),
		http.MethodGet,
		projectPath(createdProjectID),
		projectItemTemplate,
		nil,
	)
	if err == nil || strings.Contains(err.Error(), createdProjectID) {
		t.Fatalf("transport error exposed concrete path: %v", err)
	}
}

func TestInventoryRejectsDuplicatePendingEntryAndMarksExactIdentity(t *testing.T) {
	t.Parallel()

	entry := InventoryEntry{
		Target: TargetCloudCurrent,
		Type:   ResourceProject,
		Identity: ResourceIdentity{
			ID:  createdProjectID,
			Key: lifecyclePrefix,
		},
		TODO: "P0-040",
	}
	inventory := Inventory{}
	if err := inventory.Track(entry); err != nil {
		t.Fatal(err)
	}
	if err := inventory.Track(entry); err == nil {
		t.Fatal("duplicate pending entry was accepted")
	}
	if err := inventory.MarkCleaned(
		entry.Target,
		entry.Type,
		entry.Identity,
	); err != nil {
		t.Fatal(err)
	}
	if inventory.Pending() != 0 {
		t.Fatalf("Pending() = %d", inventory.Pending())
	}
	if err := inventory.MarkCleaned(
		entry.Target,
		entry.Type,
		entry.Identity,
	); err == nil {
		t.Fatal("already-cleaned identity was marked twice")
	}
}

func newLifecycleTestServer(
	t *testing.T,
) (*lifecycleMock, *httptest.Server, Config, *Client) {
	t.Helper()
	mock := &lifecycleMock{
		t:                      t,
		prefix:                 lifecyclePrefix,
		projectName:            lifecyclePrefix + " project",
		environmentName:        lifecyclePrefix + " environment",
		environmentDescription: "Terraform provider Phase 0 probe",
		segmentFlagReferences:  []segmentFlagReference{},
	}
	server := httptest.NewServer(http.HandlerFunc(mock.serveHTTP))
	cfg := testConfig(t, server.URL, syntheticToken)
	cfg.ResourcePrefix = lifecyclePrefix
	client, err := NewClient(cfg, TokenService, time.Second, server.Client())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return mock, server, cfg, client
}

func (m *lifecycleMock) serveHTTP(response http.ResponseWriter, request *http.Request) {
	m.t.Helper()
	if request.Header.Get("Authorization") != syntheticToken {
		m.t.Errorf("Authorization header did not contain the synthetic test token")
	}
	requestLine := request.Method + " " + request.URL.EscapedPath()
	m.requests = append(m.requests, requestLine)
	if request.Method != http.MethodGet {
		m.mutationPaths = append(m.mutationPaths, requestLine)
	}

	projectURL := projectPath(createdProjectID)
	duplicateProjectURL := projectPath(duplicateProjectID)
	environmentCollectionURL := environmentCollectionPath(createdProjectID)
	environmentURL := environmentPath(createdProjectID, createdEnvironmentID)
	flagCollectionURL := featureFlagCollectionPath(createdEnvironmentID)
	activeFlagKey := m.featureFlag.Key
	if activeFlagKey == "" {
		activeFlagKey = m.prefix
	}
	flagURL := featureFlagPath(createdEnvironmentID, activeFlagKey)
	flagArchiveURL := featureFlagArchivePath(createdEnvironmentID, activeFlagKey)
	flagNameURL := featureFlagNamePath(createdEnvironmentID, activeFlagKey)
	flagVariationsURL := featureFlagVariationsPath(
		createdEnvironmentID,
		activeFlagKey,
	)
	segmentCollectionURL := segmentCollectionPath(createdEnvironmentID)
	segmentURL := segmentPath(createdEnvironmentID, createdSegmentID)
	existingSharedSegmentURL := segmentPath(
		createdEnvironmentID,
		existingSharedSegmentID,
	)
	missingSegmentURL := segmentPath(
		createdEnvironmentID,
		deterministicProbeUUID(m.prefix+":segment:missing"),
	)
	duplicateSegmentURL := segmentPath(
		createdEnvironmentID,
		duplicateSegmentID,
	)
	segmentNameURL := segmentNamePath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentDescriptionURL := segmentDescriptionPath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentTargetingURL := segmentTargetingPath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentTagsURL := segmentTagsPath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentArchiveURL := segmentArchivePath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentRestoreURL := segmentRestorePath(
		createdEnvironmentID,
		createdSegmentID,
	)
	segmentFlagReferencesURL := segmentFlagReferencesPath(
		createdEnvironmentID,
		createdSegmentID,
	)

	switch {
	case request.Method == http.MethodGet && request.URL.Path == projectCollectionPath:
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.projectList(),
		)
	case request.Method == http.MethodPost && request.URL.Path == projectCollectionPath:
		var body createProjectRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.Key != m.prefix {
			m.t.Errorf("project create key = %q", body.Key)
		}
		if body.Name == "" {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusBadRequest,
				false,
				nil,
				"Required",
			)
			return
		}
		if m.projectExists {
			if m.acceptDuplicateProject {
				m.duplicateProjectExists = true
				writeLifecycleEnvelope(
					m.t,
					response,
					http.StatusOK,
					true,
					m.duplicateProject(),
				)
				return
			}
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusBadRequest,
				false,
				nil,
				"KeyHasBeenUsed",
			)
			return
		}
		m.projectExists = true
		m.projectName = body.Name
		if m.ambiguousProjectCreate {
			writeLifecycleEnvelope(m.t, response, http.StatusInternalServerError, false, nil)
			return
		}
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, m.createdProject())
	case request.Method == http.MethodGet && request.URL.Path == projectURL:
		if !m.projectExists {
			writeLifecycleEnvelope(m.t, response, http.StatusNotFound, false, nil)
			return
		}
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, m.createdProject())
	case request.Method == http.MethodPut && request.URL.Path == projectURL:
		if m.failProjectUpdate {
			writeLifecycleEnvelope(m.t, response, http.StatusForbidden, false, nil)
			return
		}
		var body updateProjectRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.ID != createdProjectID {
			m.t.Errorf("project update ID = %q", body.ID)
		}
		m.projectName = body.Name
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			map[string]interface{}{"id": createdProjectID, "name": m.projectName},
		)
	case request.Method == http.MethodDelete && request.URL.Path == projectURL:
		m.projectExists = false
		m.environmentExists = false
		m.featureFlagExists = false
		m.segmentExists = false
		m.duplicateSegmentExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodDelete && request.URL.Path == duplicateProjectURL:
		m.duplicateProjectExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPost && request.URL.Path == environmentCollectionURL:
		var body createEnvironmentRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.Key != m.prefix {
			m.t.Errorf("environment create key = %q", body.Key)
		}
		if body.Name == "" {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusBadRequest,
				false,
				nil,
				"Required",
			)
			return
		}
		if m.environmentExists {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusBadRequest,
				false,
				nil,
				"KeyHasBeenUsed",
			)
			return
		}
		m.environmentExists = true
		m.environmentName = body.Name
		m.environmentDescription = body.Description
		if m.ambiguousEnvironmentCreate {
			writeLifecycleEnvelope(m.t, response, http.StatusInternalServerError, false, nil)
			return
		}
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, m.explicitEnvironment(false))
	case request.Method == http.MethodGet && request.URL.Path == environmentURL:
		if !m.environmentExists {
			writeLifecycleEnvelope(m.t, response, http.StatusNotFound, false, nil)
			return
		}
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, m.explicitEnvironment(false))
	case request.Method == http.MethodPut && request.URL.Path == environmentURL:
		var body updateEnvironmentRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.ID != createdEnvironmentID {
			m.t.Errorf("environment update ID = %q", body.ID)
		}
		m.environmentName = body.Name
		m.environmentDescription = body.Description
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, m.explicitEnvironment(false))
	case request.Method == http.MethodDelete && request.URL.Path == environmentURL:
		m.environmentExists = false
		m.featureFlagExists = false
		m.segmentExists = false
		m.duplicateSegmentExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodGet &&
		request.URL.Path == flagCollectionURL:
		if request.URL.Query().Get("PageIndex") != "0" ||
			request.URL.Query().Get("PageSize") !=
				strconv.Itoa(featureFlagPageSize) {
			m.t.Errorf("feature-flag pagination query = %q", request.URL.RawQuery)
		}
		items := []featureFlagListItem{}
		archiveFilter := request.URL.Query().Get("IsArchived")
		includeFlag := m.featureFlagExists
		switch archiveFilter {
		case "":
			includeFlag = includeFlag && !m.featureFlag.IsArchived
		case "false":
			includeFlag = includeFlag && !m.featureFlag.IsArchived
		case "true":
			includeFlag = includeFlag && m.featureFlag.IsArchived
		default:
			m.t.Errorf(
				"feature-flag archived filter = %q",
				archiveFilter,
			)
			includeFlag = false
		}
		if includeFlag {
			items = append(items, featureFlagListItem{
				ID:  createdFeatureFlagID,
				Key: m.featureFlag.Key,
			})
		}
		total := len(items)
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			featureFlagPage{TotalCount: &total, Items: items},
		)
	case request.Method == http.MethodPost &&
		request.URL.Path == flagCollectionURL:
		var body createFeatureFlagRequest
		decodeLifecycleRequest(m.t, request, &body)
		if !strings.HasPrefix(body.Key, m.prefix) ||
			!isSupportedFeatureFlagVariationType(body.VariationType) ||
			len(body.Variations) != 2 {
			m.t.Errorf("feature-flag create body = %+v", body)
		}
		if m.featureFlagExists {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusUnprocessableEntity,
				false,
				nil,
				"KeyHasBeenUsed",
			)
			return
		}
		m.featureFlagExists = true
		m.featureFlag = featureFlagWire{
			ID:                  createdFeatureFlagID,
			EnvID:               createdEnvironmentID,
			Revision:            featureFlagRevision1,
			Name:                body.Name,
			Description:         body.Description,
			Key:                 body.Key,
			VariationType:       body.VariationType,
			Variations:          append([]featureFlagVariation{}, body.Variations...),
			TargetUsers:         []featureFlagTargetUser{},
			Rules:               []featureFlagTargetRule{},
			IsEnabled:           body.IsEnabled,
			DisabledVariationID: body.DisabledVariationID,
			Fallthrough: &featureFlagFallthrough{
				Variations: []featureFlagRollout{
					{
						ID:      body.EnabledVariationID,
						Rollout: []float64{0, 1},
					},
				},
			},
			Tags:       append([]string{}, body.Tags...),
			IsArchived: false,
		}
		if m.ambiguousFeatureFlagCreate {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusInternalServerError,
				false,
				nil,
			)
			return
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.featureFlag,
		)
	case request.Method == http.MethodGet && request.URL.Path == flagURL:
		if !m.featureFlagExists {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusNotFound,
				false,
				nil,
				"NotFound",
			)
			return
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.featureFlag,
		)
	case request.Method == http.MethodPut && request.URL.Path == flagNameURL:
		if m.failFeatureFlagNameUpdate {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusForbidden,
				false,
				nil,
				"Forbidden",
			)
			return
		}
		var body updateFeatureFlagNameRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.Name == "" {
			m.t.Error("feature-flag name update was empty")
		}
		m.featureFlag.Name = body.Name
		m.featureFlag.Revision = featureFlagRevision2
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			featureFlagRevision2,
		)
	case request.Method == http.MethodPut &&
		request.URL.Path == flagVariationsURL:
		var body updateFeatureFlagVariationsRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.Revision != m.featureFlag.Revision {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusConflict,
				false,
				nil,
				"RevisionConflict",
			)
			return
		}
		m.featureFlag.Variations = append(
			[]featureFlagVariation{},
			body.Variations...,
		)
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			featureFlagRevision2,
		)
	case request.Method == http.MethodPut &&
		request.URL.Path == flagArchiveURL:
		var body resourceChangeRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.Comment == "" {
			m.t.Error("feature-flag archive comment was empty")
		}
		m.featureFlag.IsArchived = true
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodDelete && request.URL.Path == flagURL:
		if m.requireFlagArchiveDelete && !m.featureFlag.IsArchived {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusUnprocessableEntity,
				false,
				nil,
				"CannotDeleteUnarchivedFeatureFlag",
			)
			return
		}
		m.featureFlagExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodGet &&
		request.URL.Path == segmentCollectionURL:
		if request.URL.Query().Get("PageIndex") != "0" ||
			request.URL.Query().Get("PageSize") !=
				strconv.Itoa(segmentPageSize) {
			m.t.Errorf("segment pagination query = %q", request.URL.RawQuery)
		}
		items := []segmentListItem{}
		archiveFilter := request.URL.Query().Get("IsArchived")
		includeArchiveState := func(isArchived bool) bool {
			switch archiveFilter {
			case "", "false":
				return !isArchived
			case "true":
				return isArchived
			default:
				m.t.Errorf("segment archived filter = %q", archiveFilter)
				return false
			}
		}
		if includeArchiveState(false) {
			items = append(items, segmentListItem{
				ID:  existingSharedSegmentID,
				Key: "existing-shared-segment",
			})
		}
		if m.segmentExists && includeArchiveState(m.segment.IsArchived) {
			items = append(items, segmentListItem{
				ID:  createdSegmentID,
				Key: m.prefix,
			})
		}
		if m.duplicateSegmentExists &&
			includeArchiveState(m.duplicateSegment.IsArchived) {
			items = append(items, segmentListItem{
				ID:  duplicateSegmentID,
				Key: m.prefix,
			})
		}
		total := len(items)
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			segmentPage{TotalCount: &total, Items: items},
		)
	case request.Method == http.MethodPost &&
		request.URL.Path == segmentCollectionURL:
		var body createSegmentRequest
		decodeLifecycleRequest(m.t, request, &body)
		expectedScope := "organization/" + syntheticOrganizationKey +
			":project/" + m.prefix +
			":env/" + m.prefix
		if body.Key != m.prefix ||
			body.Type != "environment-specific" ||
			len(body.Scopes) != 1 ||
			body.Scopes[0] != expectedScope {
			m.t.Errorf("segment create body = %+v", body)
		}
		if m.segmentExists {
			if m.acceptDuplicateSegment {
				m.duplicateSegmentExists = true
				m.duplicateSegment = m.segment
				m.duplicateSegment.ID = duplicateSegmentID
				writeLifecycleEnvelope(
					m.t,
					response,
					http.StatusOK,
					true,
					m.duplicateSegment,
				)
				return
			}
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusUnprocessableEntity,
				false,
				nil,
				"KeyHasBeenUsed",
			)
			return
		}
		m.segmentExists = true
		m.segment = segmentWire{
			ID:                    createdSegmentID,
			CreatedAt:             "2026-07-31T00:00:00Z",
			UpdatedAt:             "2026-07-31T00:00:00Z",
			WorkspaceID:           "synthetic-workspace-id",
			EnvID:                 createdEnvironmentID,
			Name:                  body.Name,
			Key:                   body.Key,
			Type:                  body.Type,
			Scopes:                append([]string{}, body.Scopes...),
			Description:           body.Description,
			Included:              []string{},
			Excluded:              []string{},
			Rules:                 []segmentMatchRule{},
			Tags:                  []string{},
			IsArchived:            false,
			IsEnvironmentSpecific: true,
		}
		if m.ambiguousSegmentCreate {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusInternalServerError,
				false,
				nil,
			)
			return
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.segment,
		)
	case request.Method == http.MethodGet &&
		request.URL.Path == existingSharedSegmentURL:
		scopes := []string{"organization/" + syntheticOrganizationKey}
		if m.ambiguousSegmentScopes {
			scopes = append(scopes, "organization/another-organization")
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			segmentWire{
				ID:          existingSharedSegmentID,
				EnvID:       createdEnvironmentID,
				Name:        "Existing shared segment",
				Key:         "existing-shared-segment",
				Type:        "shared",
				Scopes:      scopes,
				Description: "Existing read-only scope source",
				Included:    []string{},
				Excluded:    []string{},
				Rules:       []segmentMatchRule{},
				Tags:        []string{},
			},
		)
	case request.Method == http.MethodGet && request.URL.Path == segmentURL:
		if !m.segmentExists {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusNotFound,
				false,
				nil,
				"NotFound",
			)
			return
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.segment,
		)
	case request.Method == http.MethodGet &&
		request.URL.Path == missingSegmentURL:
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusNotFound,
			false,
			nil,
			"NotFound",
		)
	case request.Method == http.MethodGet &&
		request.URL.Path == duplicateSegmentURL:
		if !m.duplicateSegmentExists {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusNotFound,
				false,
				nil,
				"NotFound",
			)
			return
		}
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.duplicateSegment,
		)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentNameURL:
		if m.failSegmentNameUpdate {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusForbidden,
				false,
				nil,
				"Forbidden",
			)
			return
		}
		var body updateSegmentNameRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.ID != createdSegmentID || body.Name == "" {
			m.t.Errorf("segment name update = %+v", body)
		}
		m.segment.Name = body.Name
		m.segment.UpdatedAt = "2026-07-31T00:00:01Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentDescriptionURL:
		var body updateSegmentDescriptionRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.ID != createdSegmentID || body.Description == "" {
			m.t.Errorf("segment description update = %+v", body)
		}
		m.segment.Description = body.Description
		m.segment.UpdatedAt = "2026-07-31T00:00:02Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentTargetingURL:
		var body updateSegmentTargetingRequest
		decodeLifecycleRequest(m.t, request, &body)
		if len(body.Included) != 2 ||
			len(body.Excluded) != 1 ||
			len(body.Rules) != 1 ||
			len(body.Rules[0].Conditions) != 1 {
			m.t.Errorf("segment targeting update = %+v", body)
		}
		m.segment.Included = append([]string{}, body.Included...)
		m.segment.Excluded = append([]string{}, body.Excluded...)
		m.segment.Rules = append([]segmentMatchRule{}, body.Rules...)
		m.segment.UpdatedAt = "2026-07-31T00:00:03Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentTagsURL:
		var body updateSegmentTagsRequest
		decodeLifecycleRequest(m.t, request, &body)
		if body.ID != createdSegmentID || len(body.Tags) != 2 {
			m.t.Errorf("segment tags update = %+v", body)
		}
		m.segment.Tags = []string{body.Tags[1], body.Tags[0]}
		m.segment.UpdatedAt = "2026-07-31T00:00:04Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentArchiveURL:
		var body segmentResourceChangeRequest
		decodeLifecycleRequest(m.t, request, &body)
		m.segment.IsArchived = true
		m.segment.UpdatedAt = "2026-07-31T00:00:05Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodPut &&
		request.URL.Path == segmentRestoreURL:
		var body segmentResourceChangeRequest
		decodeLifecycleRequest(m.t, request, &body)
		m.segment.IsArchived = false
		m.segment.UpdatedAt = "2026-07-31T00:00:06Z"
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodGet &&
		request.URL.Path == segmentFlagReferencesURL:
		writeLifecycleEnvelope(
			m.t,
			response,
			http.StatusOK,
			true,
			m.segmentFlagReferences,
		)
	case request.Method == http.MethodDelete && request.URL.Path == segmentURL:
		if m.requireSegmentArchiveDelete && !m.segment.IsArchived {
			writeLifecycleEnvelope(
				m.t,
				response,
				http.StatusUnprocessableEntity,
				false,
				nil,
				"CannotDeleteUnArchivedSegment",
			)
			return
		}
		m.segmentExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	case request.Method == http.MethodDelete &&
		request.URL.Path == duplicateSegmentURL:
		m.duplicateSegmentExists = false
		writeLifecycleEnvelope(m.t, response, http.StatusOK, true, true)
	default:
		m.t.Errorf("unexpected request: %s", requestLine)
		writeLifecycleEnvelope(m.t, response, http.StatusNotFound, false, nil)
	}
}

func (m *lifecycleMock) projectList() []interface{} {
	existingKey := "existing-production-project"
	if m.collision {
		existingKey = m.prefix
	}
	projects := []interface{}{
		map[string]interface{}{
			"id":           existingProjectID,
			"name":         "Existing production project",
			"key":          existingKey,
			"environments": []interface{}{},
		},
	}
	if m.projectExists {
		projects = append(projects, m.createdProject())
	}
	if m.duplicateProjectExists {
		projects = append(projects, m.duplicateProject())
	}
	return projects
}

func (m *lifecycleMock) createdProject() map[string]interface{} {
	environments := []interface{}{
		m.environment(
			autoEnvironmentOneID,
			"Prod",
			"prod",
			"",
		),
		m.environment(
			autoEnvironmentTwoID,
			"Dev",
			"dev",
			"",
		),
	}
	if m.environmentExists {
		environments = append(environments, m.explicitEnvironment(true))
	}
	return map[string]interface{}{
		"id":           createdProjectID,
		"name":         m.projectName,
		"key":          m.prefix,
		"environments": environments,
	}
}

func (m *lifecycleMock) duplicateProject() map[string]interface{} {
	return map[string]interface{}{
		"id":           duplicateProjectID,
		"name":         m.projectName,
		"key":          m.prefix,
		"environments": []interface{}{},
	}
}

func (m *lifecycleMock) explicitEnvironment(includeIdentityFields bool) map[string]interface{} {
	environment := m.environment(
		createdEnvironmentID,
		m.environmentName,
		m.prefix,
		m.environmentDescription,
	)
	if !includeIdentityFields {
		delete(environment, "projectId")
		delete(environment, "key")
	}
	return environment
}

func (m *lifecycleMock) environment(
	id string,
	name string,
	key string,
	description string,
) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"projectId":   createdProjectID,
		"name":        name,
		"key":         key,
		"description": description,
		"settings": map[string]interface{}{
			"requireChangeComment": false,
		},
		"secrets": []interface{}{
			map[string]interface{}{
				"id":    "synthetic-server-secret-id",
				"name":  "Server Key",
				"type":  "Server",
				"value": lifecycleServerSecret,
			},
			map[string]interface{}{
				"id":    "synthetic-client-secret-id",
				"name":  "Client Key",
				"type":  "Client",
				"value": lifecycleClientSecret,
			},
		},
	}
}

func decodeLifecycleRequest(t *testing.T, request *http.Request, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(request.Body).Decode(target); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeLifecycleEnvelope(
	t *testing.T,
	response http.ResponseWriter,
	status int,
	success bool,
	data interface{},
	errorCodes ...string,
) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	errorsValue := []interface{}{}
	if !success {
		if len(errorCodes) == 0 {
			errorCodes = []string{"SyntheticFailure"}
		}
		for _, code := range errorCodes {
			errorsValue = append(errorsValue, map[string]interface{}{"code": code})
		}
	}
	if err := json.NewEncoder(response).Encode(map[string]interface{}{
		"success": success,
		"data":    data,
		"errors":  errorsValue,
	}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
