package probe

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"

	"github.com/featbit/terraform-provider-featbit/tools/api-probe/internal/normalization"
)

const (
	featureFlagCollectionTemplate = "/api/v1/envs/{envId}/feature-flags"
	featureFlagItemTemplate       = "/api/v1/envs/{envId}/feature-flags/{key}"
	featureFlagArchiveTemplate    = "/api/v1/envs/{envId}/feature-flags/{key}/archive"
	featureFlagNameTemplate       = "/api/v1/envs/{envId}/feature-flags/{key}/name"
	featureFlagVariationsTemplate = "/api/v1/envs/{envId}/feature-flags/{key}/variations"
	featureFlagPageSize           = 100
)

type FeatureFlagLifecycleSummary struct {
	Created                        bool                  `json:"created"`
	NameUpdated                    bool                  `json:"name_updated"`
	KeyPreserved                   bool                  `json:"key_preserved"`
	CanonicalReadsEqual            bool                  `json:"canonical_reads_equal"`
	UnrelatedFieldsPreserved       bool                  `json:"unrelated_fields_preserved"`
	VariationType                  string                `json:"variation_type"`
	VariationCount                 int                   `json:"variation_count"`
	VariationValuesCanonical       bool                  `json:"variation_values_canonical"`
	RequestedVariationIDsPreserved bool                  `json:"requested_variation_ids_preserved"`
	EnabledVariationReferenced     bool                  `json:"enabled_variation_referenced"`
	DisabledVariationPreserved     bool                  `json:"disabled_variation_preserved"`
	RevisionPresent                bool                  `json:"revision_present"`
	RevisionChangedAfterName       bool                  `json:"revision_changed_after_name"`
	Duplicate                      CompatibilityOutcome  `json:"duplicate"`
	StaleRevisionAttempted         bool                  `json:"stale_revision_attempted"`
	StaleRevision                  *CompatibilityOutcome `json:"stale_revision,omitempty"`
	PresentExactMatches            int                   `json:"present_exact_matches"`
	InitialDelete                  CompatibilityOutcome  `json:"initial_delete"`
	ArchivedBeforeDelete           bool                  `json:"archived_before_delete"`
	Archive                        *CompatibilityOutcome `json:"archive,omitempty"`
	PostDeleteRead                 CompatibilityOutcome  `json:"post_delete_read"`
	AbsentExactMatches             int                   `json:"absent_exact_matches"`
}

type featureFlagNames struct {
	name             string
	updatedName      string
	key              string
	description      string
	variationOneID   string
	variationTwoID   string
	variationOneName string
	variationTwoName string
	variationOne     string
	variationTwo     string
}

type featureFlagWire struct {
	ID                    string                  `json:"id"`
	EnvID                 string                  `json:"envId"`
	Revision              string                  `json:"revision"`
	Name                  string                  `json:"name"`
	Description           string                  `json:"description"`
	Key                   string                  `json:"key"`
	VariationType         string                  `json:"variationType"`
	Variations            []featureFlagVariation  `json:"variations"`
	TargetUsers           []featureFlagTargetUser `json:"targetUsers"`
	Rules                 []featureFlagTargetRule `json:"rules"`
	IsEnabled             bool                    `json:"isEnabled"`
	DisabledVariationID   string                  `json:"disabledVariationId"`
	Fallthrough           *featureFlagFallthrough `json:"fallthrough"`
	ExptIncludeAllTargets bool                    `json:"exptIncludeAllTargets"`
	Tags                  []string                `json:"tags"`
	IsArchived            bool                    `json:"isArchived"`
}

type featureFlagVariation struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type featureFlagTargetUser struct {
	KeyIDs      []string `json:"keyIds"`
	VariationID string   `json:"variationId"`
}

type featureFlagTargetRule struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	DispatchKey    string                 `json:"dispatchKey"`
	IncludedInExpt bool                   `json:"includedInExpt"`
	Conditions     []featureFlagCondition `json:"conditions"`
	Variations     []featureFlagRollout   `json:"variations"`
}

type featureFlagCondition struct {
	ID       string `json:"id"`
	Property string `json:"property"`
	Operator string `json:"op"`
	Value    string `json:"value"`
}

type featureFlagFallthrough struct {
	DispatchKey    string               `json:"dispatchKey"`
	IncludedInExpt bool                 `json:"includedInExpt"`
	Variations     []featureFlagRollout `json:"variations"`
}

type featureFlagRollout struct {
	ID          string    `json:"id"`
	Rollout     []float64 `json:"rollout"`
	ExptRollout float64   `json:"exptRollout"`
}

type featureFlagListItem struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type featureFlagPage struct {
	TotalCount *int                  `json:"totalCount"`
	Items      []featureFlagListItem `json:"items"`
}

type createFeatureFlagRequest struct {
	Name                string                 `json:"name"`
	Key                 string                 `json:"key"`
	IsEnabled           bool                   `json:"isEnabled"`
	Description         string                 `json:"description"`
	VariationType       string                 `json:"variationType"`
	Variations          []featureFlagVariation `json:"variations"`
	EnabledVariationID  string                 `json:"enabledVariationId"`
	DisabledVariationID string                 `json:"disabledVariationId"`
	Tags                []string               `json:"tags"`
}

type updateFeatureFlagNameRequest struct {
	Comment string `json:"comment,omitempty"`
	EnvID   string `json:"envId,omitempty"`
	Key     string `json:"key,omitempty"`
	Name    string `json:"name"`
}

type updateFeatureFlagVariationsRequest struct {
	Comment    string                 `json:"comment,omitempty"`
	EnvID      string                 `json:"envId,omitempty"`
	Key        string                 `json:"key,omitempty"`
	Revision   string                 `json:"revision"`
	Variations []featureFlagVariation `json:"variations"`
}

type resourceChangeRequest struct {
	Comment string `json:"comment,omitempty"`
}

func lifecycleProbeFeatureFlag(
	ctx context.Context,
	cfg Config,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	inventory *Inventory,
	inventoryPath string,
	projectID string,
	environmentID string,
	prefix string,
	variationType string,
	todo string,
	compatibilityChecks bool,
) error {
	if report.FeatureFlag == nil {
		return errors.New("feature-flag lifecycle summary is required")
	}
	if environmentID == "" {
		return errors.New("feature-flag lifecycle requires an owned environment")
	}

	names, err := featureFlagNamesForType(prefix, variationType)
	if err != nil {
		return err
	}
	preflight, err := lifecycleFindFeatureFlagByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if err != nil {
		return err
	}
	if preflight.Count != 0 {
		return errors.New(
			"exact feature-flag key already exists; refusing to mutate a pre-existing object",
		)
	}

	createBody := createFeatureFlagRequest{
		Name:          names.name,
		Key:           names.key,
		IsEnabled:     true,
		Description:   names.description,
		VariationType: variationType,
		Variations: []featureFlagVariation{
			{
				ID:    names.variationOneID,
				Name:  names.variationOneName,
				Value: names.variationOne,
			},
			{
				ID:    names.variationTwoID,
				Name:  names.variationTwoName,
				Value: names.variationTwo,
			},
		},
		EnabledVariationID:  names.variationOneID,
		DisabledVariationID: names.variationTwoID,
		Tags:                []string{"terraform-phase0"},
	}
	createResult, createErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		featureFlagCollectionPath(environmentID),
		featureFlagCollectionTemplate,
		createBody,
	)
	createClassification := Classify(createResult.Observation, createErr)
	flagIdentity := ResourceIdentity{
		EnvironmentID: environmentID,
		ProjectID:     projectID,
		Key:           names.key,
	}
	tracked := false
	track := func() error {
		if tracked {
			return nil
		}
		if err := inventory.Track(InventoryEntry{
			Target:   cfg.Target,
			Type:     ResourceFlag,
			Identity: flagIdentity,
			TODO:     todo,
		}); err != nil {
			return err
		}
		if err := inventory.Save(inventoryPath); err != nil {
			return err
		}
		tracked = true
		report.Cleanup.PendingInventory = inventory.Pending()
		return nil
	}
	if createClassification == ClassificationSuccess {
		if err := track(); err != nil {
			return err
		}
	}

	afterCreateMatch, lookupErr := lifecycleFindFeatureFlagByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if lookupErr != nil {
		return errors.New(
			"feature-flag create could not be reconciled by all-page exact key",
		)
	}
	report.FeatureFlag.PresentExactMatches = afterCreateMatch.Count
	if createClassification != ClassificationSuccess &&
		afterCreateMatch.Count > 0 {
		if err := track(); err != nil {
			return err
		}
		report.Workarounds = append(
			report.Workarounds,
			"feature_flag_create_all_page_exact_key_reconciliation",
		)
	}
	if afterCreateMatch.Count != 1 {
		if createErr != nil && afterCreateMatch.Count == 0 {
			return createErr
		}
		return fmt.Errorf(
			"feature-flag create classification %s; all-page exact-key reconciliation found %d matches",
			createClassification,
			afterCreateMatch.Count,
		)
	}
	if !tracked {
		return errors.New(
			"feature-flag exact identity appeared without a safely classified create",
		)
	}
	report.FeatureFlag.Created = true

	beforeName, err := lifecycleReadFeatureFlag(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if err != nil {
		return err
	}
	if beforeName.Key != names.key ||
		(beforeName.EnvID != "" && beforeName.EnvID != environmentID) {
		return errors.New("created feature flag did not preserve exact scoped identity")
	}

	if compatibilityChecks {
		duplicateResult, duplicateErr := lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodPost,
			featureFlagCollectionPath(environmentID),
			featureFlagCollectionTemplate,
			createBody,
		)
		afterDuplicateMatch, duplicateLookupErr := lifecycleFindFeatureFlagByKeyAnyState(
			ctx,
			client,
			report,
			environmentID,
			names.key,
		)
		if duplicateLookupErr != nil {
			return errors.New(
				"duplicate feature-flag request could not be reconciled by all-page exact key",
			)
		}
		report.FeatureFlag.Duplicate = compatibilityOutcome(
			duplicateResult,
			duplicateErr,
			afterDuplicateMatch.Count,
		)
		if afterDuplicateMatch.Count != 1 {
			return errors.New(
				"duplicate feature-flag request changed the exact-key identity set; parent cleanup is pending",
			)
		}
		if duplicateErr != nil ||
			report.FeatureFlag.Duplicate.Classification == ClassificationSuccess {
			report.Workarounds = append(
				report.Workarounds,
				"feature_flag_duplicate_all_page_exact_key_reconciliation",
			)
		}
	}

	beforeUnrelated := canonicalFeatureFlag(beforeName)
	beforeUnrelated.Name = ""
	beforeUnrelated.Revision = ""
	updateResult, updateErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		featureFlagNamePath(environmentID, names.key),
		featureFlagNameTemplate,
		updateFeatureFlagNameRequest{
			Comment: "Terraform provider Phase 0 probe",
			Name:    names.updatedName,
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		featureFlagNameTemplate,
		updateResult,
		updateErr,
	); err != nil {
		return err
	}
	report.FeatureFlag.NameUpdated = true

	afterName, err := lifecycleReadFeatureFlag(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if err != nil {
		return err
	}
	afterUnrelated := canonicalFeatureFlag(afterName)
	afterUnrelated.Name = ""
	afterUnrelated.Revision = ""
	report.FeatureFlag.UnrelatedFieldsPreserved = reflect.DeepEqual(
		beforeUnrelated,
		afterUnrelated,
	)
	report.FeatureFlag.KeyPreserved = afterName.Key == names.key
	report.FeatureFlag.RevisionChangedAfterName =
		beforeName.Revision != "" &&
			afterName.Revision != "" &&
			beforeName.Revision != afterName.Revision
	if afterName.Name != names.updatedName ||
		!report.FeatureFlag.KeyPreserved ||
		!report.FeatureFlag.UnrelatedFieldsPreserved {
		return errors.New(
			"feature-flag name update changed unrelated canonical fields",
		)
	}

	stableFlag := afterName
	if compatibilityChecks && report.FeatureFlag.RevisionChangedAfterName {
		report.FeatureFlag.StaleRevisionAttempted = true
		staleResult, staleErr := lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodPut,
			featureFlagVariationsPath(environmentID, names.key),
			featureFlagVariationsTemplate,
			updateFeatureFlagVariationsRequest{
				Comment:    "Terraform provider Phase 0 stale-revision probe",
				Revision:   beforeName.Revision,
				Variations: append([]featureFlagVariation{}, afterName.Variations...),
			},
		)
		staleOutcome := compatibilityOutcome(staleResult, staleErr, 1)
		report.FeatureFlag.StaleRevision = &staleOutcome
		report.Workarounds = append(
			report.Workarounds,
			"feature_flag_stale_revision_same_value_probe",
		)

		afterStale, readErr := lifecycleReadFeatureFlag(
			ctx,
			client,
			report,
			environmentID,
			names.key,
		)
		if readErr != nil {
			return readErr
		}
		staleUnrelated := canonicalFeatureFlag(afterStale)
		staleUnrelated.Revision = ""
		expectedUnrelated := canonicalFeatureFlag(afterName)
		expectedUnrelated.Revision = ""
		if !reflect.DeepEqual(staleUnrelated, expectedUnrelated) {
			return errors.New(
				"same-value stale-revision probe changed canonical feature-flag state",
			)
		}
		stableFlag = afterStale
	}

	secondStableRead, err := lifecycleReadFeatureFlag(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if err != nil {
		return err
	}
	report.FeatureFlag.CanonicalReadsEqual = reflect.DeepEqual(
		canonicalFeatureFlag(stableFlag),
		canonicalFeatureFlag(secondStableRead),
	)
	if !report.FeatureFlag.CanonicalReadsEqual {
		return errors.New("feature-flag repeated canonical Reads did not converge")
	}
	if err := recordFeatureFlagShape(
		report.FeatureFlag,
		secondStableRead,
		names,
		variationType,
	); err != nil {
		return err
	}

	deleteResult, deleteErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodDelete,
		featureFlagPath(environmentID, names.key),
		featureFlagItemTemplate,
		nil,
	)
	report.FeatureFlag.InitialDelete = compatibilityOutcome(
		deleteResult,
		deleteErr,
		-1,
	)
	if observationHasErrorCode(
		deleteResult.Observation,
		"CannotDeleteUnarchivedFeatureFlag",
	) {
		archiveResult, archiveErr := lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodPut,
			featureFlagArchivePath(environmentID, names.key),
			featureFlagArchiveTemplate,
			resourceChangeRequest{
				Comment: "Terraform provider Phase 0 delete prerequisite",
			},
		)
		archiveOutcome := compatibilityOutcome(archiveResult, archiveErr, 1)
		report.FeatureFlag.Archive = &archiveOutcome
		if err := requireLifecycleSuccess(
			http.MethodPut,
			featureFlagArchiveTemplate,
			archiveResult,
			archiveErr,
		); err != nil {
			return err
		}
		report.FeatureFlag.ArchivedBeforeDelete = true
		report.Workarounds = append(
			report.Workarounds,
			"feature_flag_archive_before_delete",
		)
		deleteResult, deleteErr = lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodDelete,
			featureFlagPath(environmentID, names.key),
			featureFlagItemTemplate,
			nil,
		)
	}
	postDeleteResult, postDeleteErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		featureFlagPath(environmentID, names.key),
		featureFlagItemTemplate,
		nil,
	)
	report.FeatureFlag.PostDeleteRead = compatibilityOutcome(
		postDeleteResult,
		postDeleteErr,
		-1,
	)
	if report.FeatureFlag.PostDeleteRead.Classification != ClassificationSuccess {
		report.Workarounds = append(
			report.Workarounds,
			"feature_flag_post_delete_all_page_exact_key_fallback",
		)
	}

	afterDeleteMatch, afterDeleteLookupErr := lifecycleFindFeatureFlagByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if afterDeleteLookupErr != nil {
		return errors.New(
			"feature-flag post-delete all-page exact-key fallback failed",
		)
	}
	report.FeatureFlag.AbsentExactMatches = afterDeleteMatch.Count
	report.FeatureFlag.PostDeleteRead.ExactMatchCount = afterDeleteMatch.Count
	if report.FeatureFlag.PostDeleteRead.Classification == ClassificationSuccess {
		return errors.New(
			"feature-flag direct Read remained present after Delete; preserving cleanup state",
		)
	}
	if afterDeleteMatch.Count != 0 {
		if deleteErr != nil {
			return deleteErr
		}
		return errors.New(
			"feature-flag Delete returned but exact identity remains; parent cleanup is pending",
		)
	}
	if err := requireDeleteOrVerifiedAbsence(
		http.MethodDelete,
		featureFlagItemTemplate,
		deleteResult,
		deleteErr,
		report,
		"feature_flag_delete_verified_by_all_page_exact_absence",
	); err != nil {
		return err
	}
	if err := inventory.MarkCleaned(cfg.Target, ResourceFlag, flagIdentity); err != nil {
		return err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return err
	}
	report.Cleanup.FeatureFlagAbsent = true
	report.Cleanup.PendingInventory = inventory.Pending()
	return nil
}

func lifecycleFindFeatureFlagByKey(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedKey string,
) (ExactMatch[featureFlagListItem], error) {
	return lifecycleFindFeatureFlagByKeyState(
		ctx,
		client,
		report,
		environmentID,
		expectedKey,
		nil,
	)
}

func lifecycleFindFeatureFlagByKeyAnyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedKey string,
) (ExactMatch[featureFlagListItem], error) {
	active := false
	activeMatch, err := lifecycleFindFeatureFlagByKeyState(
		ctx,
		client,
		report,
		environmentID,
		expectedKey,
		&active,
	)
	if err != nil {
		return ExactMatch[featureFlagListItem]{}, err
	}
	archived := true
	archivedMatch, err := lifecycleFindFeatureFlagByKeyState(
		ctx,
		client,
		report,
		environmentID,
		expectedKey,
		&archived,
	)
	if err != nil {
		return ExactMatch[featureFlagListItem]{}, err
	}
	result := ExactMatch[featureFlagListItem]{
		Count: activeMatch.Count + archivedMatch.Count,
		Items: append(
			append([]featureFlagListItem{}, activeMatch.Items...),
			archivedMatch.Items...,
		),
	}
	if result.Count > 0 {
		result.Item = result.Items[0]
	}
	return result, nil
}

func lifecycleFindFeatureFlagByKeyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedKey string,
	archived *bool,
) (ExactMatch[featureFlagListItem], error) {
	return FindExactAcrossAllPages(
		ctx,
		featureFlagPageSize,
		func(
			ctx context.Context,
			pageIndex int,
			pageSize int,
		) (Page[featureFlagListItem], error) {
			query := url.Values{}
			query.Set("PageIndex", strconv.Itoa(pageIndex))
			query.Set("PageSize", strconv.Itoa(pageSize))
			if archived != nil {
				query.Set("IsArchived", strconv.FormatBool(*archived))
			}
			result, requestErr := lifecycleRequest(
				ctx,
				client,
				report,
				http.MethodGet,
				featureFlagCollectionPath(environmentID)+"?"+query.Encode(),
				featureFlagCollectionTemplate,
				nil,
			)
			if err := requireLifecycleSuccess(
				http.MethodGet,
				featureFlagCollectionTemplate,
				result,
				requestErr,
			); err != nil {
				return Page[featureFlagListItem]{}, err
			}
			page, err := decodeLifecycleData[featureFlagPage](result)
			if err != nil {
				return Page[featureFlagListItem]{}, err
			}
			return Page[featureFlagListItem]{
				Items:      page.Items,
				TotalCount: page.TotalCount,
			}, nil
		},
		func(item featureFlagListItem) string { return item.Key },
		expectedKey,
		nil,
	)
}

func lifecycleReadFeatureFlag(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	key string,
) (featureFlagWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		featureFlagPath(environmentID, key),
		featureFlagItemTemplate,
		nil,
	)
	if err := requireLifecycleSuccess(
		http.MethodGet,
		featureFlagItemTemplate,
		result,
		requestErr,
	); err != nil {
		return featureFlagWire{}, err
	}
	return decodeLifecycleData[featureFlagWire](result)
}

func featureFlagNamesForPrefix(prefix string) featureFlagNames {
	names, _ := featureFlagNamesForType(prefix, "boolean")
	return names
}

func featureFlagNamesForType(
	prefix string,
	variationType string,
) (featureFlagNames, error) {
	key := prefix
	variationOneName := "Primary"
	variationTwoName := "Secondary"
	variationOne := ""
	variationTwo := ""
	switch variationType {
	case "boolean":
		variationOneName = "True"
		variationTwoName = "False"
		variationOne = "true"
		variationTwo = "false"
	case "string":
		key += "-string"
		variationOne = "alpha"
		variationTwo = "beta"
	case "number":
		key += "-number"
		variationOne = "9007199254740993.000"
		variationTwo = "3.1400"
	case "json":
		key += "-json"
		variationOne = `{"z":2,"a":[true,1.0,9007199254740993]}`
		variationTwo = `{"kind":"secondary","count":0}`
	default:
		return featureFlagNames{}, fmt.Errorf(
			"unsupported feature-flag variation type %q",
			variationType,
		)
	}
	return featureFlagNames{
		name:             key + " flag",
		updatedName:      key + " flag updated",
		key:              key,
		description:      "Terraform provider Phase 0 probe",
		variationOneID:   deterministicProbeUUID(key + ":variation:primary"),
		variationTwoID:   deterministicProbeUUID(key + ":variation:secondary"),
		variationOneName: variationOneName,
		variationTwoName: variationTwoName,
		variationOne:     variationOne,
		variationTwo:     variationTwo,
	}, nil
}

func isSupportedFeatureFlagVariationType(variationType string) bool {
	switch variationType {
	case "boolean", "string", "number", "json":
		return true
	default:
		return false
	}
}

func deterministicProbeUUID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	value := append([]byte{}, sum[:16]...)
	value[6] = (value[6] & 0x0f) | 0x50
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	)
}

func recordFeatureFlagShape(
	summary *FeatureFlagLifecycleSummary,
	flag featureFlagWire,
	names featureFlagNames,
	expectedVariationType string,
) error {
	summary.VariationType = flag.VariationType
	summary.VariationCount = len(flag.Variations)
	summary.RequestedVariationIDsPreserved =
		len(flag.Variations) == 2 &&
			flag.Variations[0].ID == names.variationOneID &&
			flag.Variations[1].ID == names.variationTwoID
	summary.EnabledVariationReferenced = false
	if flag.Fallthrough != nil {
		for _, variation := range flag.Fallthrough.Variations {
			if variation.ID == names.variationOneID {
				summary.EnabledVariationReferenced = true
				break
			}
		}
	}
	summary.DisabledVariationPreserved =
		flag.DisabledVariationID == names.variationTwoID
	summary.RevisionPresent = flag.Revision != ""
	summary.VariationValuesCanonical = featureFlagVariationValuesMatch(
		expectedVariationType,
		[]featureFlagVariation{
			{
				ID:    names.variationOneID,
				Name:  names.variationOneName,
				Value: names.variationOne,
			},
			{
				ID:    names.variationTwoID,
				Name:  names.variationTwoName,
				Value: names.variationTwo,
			},
		},
		flag.Variations,
	)
	if summary.VariationType != expectedVariationType ||
		summary.VariationCount != 2 ||
		!summary.RequestedVariationIDsPreserved ||
		!summary.EnabledVariationReferenced ||
		!summary.DisabledVariationPreserved ||
		!summary.RevisionPresent ||
		!summary.VariationValuesCanonical {
		return errors.New(
			"feature-flag Read did not preserve the requested type, variation identity, or canonical values",
		)
	}
	return nil
}

func featureFlagVariationValuesMatch(
	variationType string,
	expected []featureFlagVariation,
	observed []featureFlagVariation,
) bool {
	if len(expected) != len(observed) {
		return false
	}
	for index := range expected {
		if expected[index].ID != observed[index].ID ||
			expected[index].Name != observed[index].Name {
			return false
		}
		expectedValue, err := normalization.CanonicalVariationValue(
			variationType,
			expected[index].Value,
		)
		if err != nil {
			return false
		}
		observedValue, err := normalization.CanonicalVariationValue(
			variationType,
			observed[index].Value,
		)
		if err != nil || expectedValue != observedValue {
			return false
		}
	}
	return true
}

func canonicalFeatureFlag(input featureFlagWire) featureFlagWire {
	output := input
	output.Variations = append([]featureFlagVariation{}, input.Variations...)
	output.Tags = canonicalStringSet(input.Tags)
	output.TargetUsers = make(
		[]featureFlagTargetUser,
		len(input.TargetUsers),
	)
	for index, target := range input.TargetUsers {
		output.TargetUsers[index] = target
		output.TargetUsers[index].KeyIDs = canonicalStringSet(target.KeyIDs)
	}
	sort.SliceStable(output.TargetUsers, func(left, right int) bool {
		return output.TargetUsers[left].VariationID <
			output.TargetUsers[right].VariationID
	})
	output.Rules = make([]featureFlagTargetRule, len(input.Rules))
	for index, rule := range input.Rules {
		output.Rules[index] = rule
		output.Rules[index].Conditions = append(
			[]featureFlagCondition{},
			rule.Conditions...,
		)
		output.Rules[index].Variations = canonicalFeatureFlagRollouts(
			rule.Variations,
		)
	}
	if input.Fallthrough != nil {
		canonicalFallthrough := *input.Fallthrough
		canonicalFallthrough.Variations = canonicalFeatureFlagRollouts(
			input.Fallthrough.Variations,
		)
		output.Fallthrough = &canonicalFallthrough
	}
	return output
}

func canonicalFeatureFlagRollouts(
	input []featureFlagRollout,
) []featureFlagRollout {
	output := make([]featureFlagRollout, len(input))
	for index, rollout := range input {
		output[index] = rollout
		output[index].Rollout = append([]float64{}, rollout.Rollout...)
	}
	return output
}

func canonicalStringSet(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}
	values := append([]string{}, input...)
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func featureFlagCollectionPath(environmentID string) string {
	return "/api/v1/envs/" + url.PathEscape(environmentID) + "/feature-flags"
}

func featureFlagPath(environmentID string, key string) string {
	return featureFlagCollectionPath(environmentID) + "/" + url.PathEscape(key)
}

func featureFlagNamePath(environmentID string, key string) string {
	return featureFlagPath(environmentID, key) + "/name"
}

func featureFlagArchivePath(environmentID string, key string) string {
	return featureFlagPath(environmentID, key) + "/archive"
}

func featureFlagVariationsPath(environmentID string, key string) string {
	return featureFlagPath(environmentID, key) + "/variations"
}

func observationHasErrorCode(observation Observation, expected string) bool {
	for _, code := range observation.ErrorCodes {
		if code == expected {
			return true
		}
	}
	return false
}
