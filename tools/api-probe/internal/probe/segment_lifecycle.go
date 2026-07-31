package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

const (
	segmentCollectionTemplate     = "/api/v1/envs/{envId}/segments"
	segmentItemTemplate           = "/api/v1/envs/{envId}/segments/{segmentId}"
	segmentNameTemplate           = "/api/v1/envs/{envId}/segments/{segmentId}/name"
	segmentDescriptionTemplate    = "/api/v1/envs/{envId}/segments/{segmentId}/description"
	segmentTargetingTemplate      = "/api/v1/envs/{envId}/segments/{segmentId}/targeting"
	segmentTagsTemplate           = "/api/v1/envs/{envId}/segments/{segmentId}/tags"
	segmentArchiveTemplate        = "/api/v1/envs/{envId}/segments/{segmentId}/archive"
	segmentRestoreTemplate        = "/api/v1/envs/{envId}/segments/{segmentId}/restore"
	segmentFlagReferencesTemplate = "/api/v1/envs/{envId}/segments/{segmentId}/flag-references"
	segmentPageSize               = 100
)

type SegmentLifecycleSummary struct {
	Created                  bool                  `json:"created"`
	NameUpdated              bool                  `json:"name_updated"`
	DescriptionUpdated       bool                  `json:"description_updated"`
	TargetingUpdated         bool                  `json:"targeting_updated"`
	TagsUpdated              bool                  `json:"tags_updated"`
	Archived                 bool                  `json:"archived"`
	Restored                 bool                  `json:"restored"`
	KeyPreserved             bool                  `json:"key_preserved"`
	TypePreserved            bool                  `json:"type_preserved"`
	ScopesPreserved          bool                  `json:"scopes_preserved"`
	EnvironmentSpecific      bool                  `json:"environment_specific"`
	CanonicalReadsEqual      bool                  `json:"canonical_reads_equal"`
	UnrelatedFieldsPreserved bool                  `json:"unrelated_fields_preserved"`
	IncludedCount            int                   `json:"included_count"`
	ExcludedCount            int                   `json:"excluded_count"`
	RuleCount                int                   `json:"rule_count"`
	ConditionCount           int                   `json:"condition_count"`
	TagCount                 int                   `json:"tag_count"`
	FlagReferenceCount       int                   `json:"flag_reference_count"`
	Duplicate                CompatibilityOutcome  `json:"duplicate"`
	PresentExactKeyMatches   int                   `json:"present_exact_key_matches"`
	PresentExactIDMatches    int                   `json:"present_exact_id_matches"`
	InitialDelete            CompatibilityOutcome  `json:"initial_delete"`
	ArchivedBeforeDelete     bool                  `json:"archived_before_delete"`
	DeleteArchive            *CompatibilityOutcome `json:"delete_archive,omitempty"`
	PostDeleteRead           CompatibilityOutcome  `json:"post_delete_read"`
	AbsentExactIDMatches     int                   `json:"absent_exact_id_matches"`
	AbsentExactKeyMatches    int                   `json:"absent_exact_key_matches"`
}

type segmentNames struct {
	name               string
	updatedName        string
	key                string
	description        string
	updatedDescription string
	includedUsers      []string
	excludedUsers      []string
	tags               []string
	ruleID             string
	ruleName           string
	conditionID        string
	conditionValue     string
}

type segmentWire struct {
	ID                    string             `json:"id"`
	CreatedAt             string             `json:"createdAt"`
	UpdatedAt             string             `json:"updatedAt"`
	WorkspaceID           string             `json:"workspaceId"`
	EnvID                 string             `json:"envId"`
	Name                  string             `json:"name"`
	Key                   string             `json:"key"`
	Type                  string             `json:"type"`
	Scopes                []string           `json:"scopes"`
	Description           string             `json:"description"`
	Included              []string           `json:"included"`
	Excluded              []string           `json:"excluded"`
	Rules                 []segmentMatchRule `json:"rules"`
	Tags                  []string           `json:"tags"`
	IsArchived            bool               `json:"isArchived"`
	IsEnvironmentSpecific bool               `json:"isEnvironmentSpecific"`
}

type segmentMatchRule struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Conditions []segmentCondition `json:"conditions"`
}

type segmentCondition struct {
	ID       string `json:"id"`
	Property string `json:"property"`
	Operator string `json:"op"`
	Value    string `json:"value"`
}

type segmentListItem struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type segmentPage struct {
	TotalCount *int              `json:"totalCount"`
	Items      []segmentListItem `json:"items"`
}

type segmentFlagReference struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type createSegmentRequest struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`
}

type updateSegmentNameRequest struct {
	Comment string `json:"comment,omitempty"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type updateSegmentDescriptionRequest struct {
	Comment     string `json:"comment,omitempty"`
	ID          string `json:"id"`
	Description string `json:"description"`
}

type updateSegmentTargetingRequest struct {
	Comment  string             `json:"comment,omitempty"`
	Included []string           `json:"included"`
	Excluded []string           `json:"excluded"`
	Rules    []segmentMatchRule `json:"rules"`
}

type updateSegmentTagsRequest struct {
	Comment string   `json:"comment,omitempty"`
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
}

type segmentResourceChangeRequest struct {
	Comment string `json:"comment,omitempty"`
}

func lifecycleProbeSegment(
	ctx context.Context,
	cfg Config,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	inventory *Inventory,
	inventoryPath string,
	projectID string,
	environmentID string,
	projectKey string,
	environmentKey string,
	compatibilityChecks bool,
) error {
	if report.Segment == nil {
		return errors.New("segment lifecycle summary is required")
	}
	if environmentID == "" {
		return errors.New("segment lifecycle requires an owned environment")
	}

	names := segmentNamesForPrefix(projectKey)
	preflight, err := lifecycleFindSegmentByKeyAnyState(
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
			"exact segment key already exists; refusing to mutate a pre-existing object",
		)
	}

	environmentScope, err := lifecycleResolveSegmentEnvironmentScope(
		ctx,
		client,
		report,
		environmentID,
		projectKey,
		environmentKey,
	)
	if err != nil {
		return err
	}
	createBody := createSegmentRequest{
		Type:        "environment-specific",
		Name:        names.name,
		Key:         names.key,
		Description: names.description,
		Scopes:      []string{environmentScope},
	}
	createResult, createErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		segmentCollectionPath(environmentID),
		segmentCollectionTemplate,
		createBody,
	)
	createClassification := Classify(createResult.Observation, createErr)
	trackedIDs := map[string]struct{}{}
	track := func(segmentID string, todo string) error {
		if segmentID == "" {
			return errors.New("segment response lacked a valid cleanup identity")
		}
		if _, exists := trackedIDs[segmentID]; exists {
			return nil
		}
		identity := ResourceIdentity{
			ID:            segmentID,
			Key:           names.key,
			ProjectID:     projectID,
			EnvironmentID: environmentID,
		}
		if err := inventory.Track(InventoryEntry{
			Target:   cfg.Target,
			Type:     ResourceSegment,
			Identity: identity,
			TODO:     todo,
		}); err != nil {
			return err
		}
		if err := inventory.Save(inventoryPath); err != nil {
			return err
		}
		trackedIDs[segmentID] = struct{}{}
		report.Cleanup.PendingInventory = inventory.Pending()
		return nil
	}

	var responseSegment segmentWire
	if createClassification == ClassificationSuccess {
		decoded, decodeErr := decodeLifecycleData[segmentWire](createResult)
		if decodeErr == nil && decoded.ID != "" {
			responseSegment = decoded
			if err := track(decoded.ID, "P0-060"); err != nil {
				return err
			}
		}
	}

	afterCreateMatch, lookupErr := lifecycleFindSegmentByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if lookupErr != nil {
		return errors.New(
			"segment create could not be reconciled by all-page exact key",
		)
	}
	report.Segment.PresentExactKeyMatches = afterCreateMatch.Count
	if afterCreateMatch.Count > 0 {
		for _, item := range afterCreateMatch.Items {
			if err := track(item.ID, "P0-060"); err != nil {
				return err
			}
		}
	}
	if createClassification != ClassificationSuccess &&
		afterCreateMatch.Count > 0 {
		report.Workarounds = append(
			report.Workarounds,
			"segment_create_all_page_exact_key_reconciliation",
		)
	}
	if createClassification == ClassificationSuccess &&
		responseSegment.ID == "" &&
		afterCreateMatch.Count > 0 {
		report.Workarounds = append(
			report.Workarounds,
			"segment_create_all_page_exact_key_reconciliation",
		)
	}
	if afterCreateMatch.Count != 1 {
		if createErr != nil && afterCreateMatch.Count == 0 {
			return createErr
		}
		return fmt.Errorf(
			"segment create classification %s; all-page exact-key reconciliation found %d matches",
			createClassification,
			afterCreateMatch.Count,
		)
	}
	segmentID := afterCreateMatch.Item.ID
	if responseSegment.ID != "" && responseSegment.ID != segmentID {
		return errors.New(
			"segment create response and exact-key list returned different identities; cleanup is pending",
		)
	}
	if _, tracked := trackedIDs[segmentID]; !tracked {
		return errors.New(
			"segment exact identity appeared without a safely classified create",
		)
	}
	report.Segment.Created = true

	beforeDuplicate, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	if beforeDuplicate.ID != segmentID ||
		beforeDuplicate.Key != names.key ||
		(beforeDuplicate.EnvID != "" &&
			beforeDuplicate.EnvID != environmentID) {
		return errors.New("created segment did not preserve exact scoped identity")
	}

	current := beforeDuplicate
	if compatibilityChecks {
		duplicateResult, duplicateErr := lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodPost,
			segmentCollectionPath(environmentID),
			segmentCollectionTemplate,
			createBody,
		)
		if duplicateErr == nil &&
			Classify(duplicateResult.Observation, nil) == ClassificationSuccess {
			duplicate, decodeErr := decodeLifecycleData[segmentWire](duplicateResult)
			if decodeErr == nil && duplicate.ID != "" &&
				duplicate.ID != segmentID {
				if err := track(duplicate.ID, "P0-031"); err != nil {
					return err
				}
			}
		}
		afterDuplicateMatch, duplicateLookupErr :=
			lifecycleFindSegmentByKeyAnyState(
				ctx,
				client,
				report,
				environmentID,
				names.key,
			)
		if duplicateLookupErr != nil {
			return errors.New(
				"duplicate segment request could not be reconciled by all-page exact key",
			)
		}
		for _, item := range afterDuplicateMatch.Items {
			if item.ID != segmentID {
				if err := track(item.ID, "P0-031"); err != nil {
					return err
				}
			}
		}
		report.Segment.Duplicate = compatibilityOutcome(
			duplicateResult,
			duplicateErr,
			afterDuplicateMatch.Count,
		)
		if afterDuplicateMatch.Count != 1 ||
			afterDuplicateMatch.Item.ID != segmentID {
			return errors.New(
				"duplicate segment request changed the exact-key identity set; cleanup is pending",
			)
		}
		afterDuplicate, readErr := lifecycleReadSegment(
			ctx,
			client,
			report,
			environmentID,
			segmentID,
		)
		if readErr != nil {
			return readErr
		}
		if !reflect.DeepEqual(
			canonicalSegment(beforeDuplicate),
			canonicalSegment(afterDuplicate),
		) {
			return errors.New("duplicate segment request changed canonical state")
		}
		if duplicateErr != nil ||
			report.Segment.Duplicate.Classification == ClassificationSuccess {
			report.Workarounds = append(
				report.Workarounds,
				"segment_duplicate_all_page_exact_key_reconciliation",
			)
		}
		current = afterDuplicate
	}
	unrelatedPreserved := true

	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentNamePath(environmentID, segmentID),
		segmentNameTemplate,
		updateSegmentNameRequest{
			Comment: "Terraform provider Phase 0 probe",
			ID:      segmentID,
			Name:    names.updatedName,
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentNameTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterName, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther := canonicalSegment(current)
	afterOther := canonicalSegment(afterName)
	beforeOther.Name = ""
	afterOther.Name = ""
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if afterName.Name != names.updatedName || !unrelatedPreserved {
		return errors.New("segment name update changed unrelated canonical fields")
	}
	report.Segment.NameUpdated = true
	current = afterName

	result, requestErr = lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentDescriptionPath(environmentID, segmentID),
		segmentDescriptionTemplate,
		updateSegmentDescriptionRequest{
			Comment:     "Terraform provider Phase 0 probe",
			ID:          segmentID,
			Description: names.updatedDescription,
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentDescriptionTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterDescription, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther = canonicalSegment(current)
	afterOther = canonicalSegment(afterDescription)
	beforeOther.Description = ""
	afterOther.Description = ""
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if afterDescription.Description != names.updatedDescription ||
		!unrelatedPreserved {
		return errors.New(
			"segment description update changed unrelated canonical fields",
		)
	}
	report.Segment.DescriptionUpdated = true
	current = afterDescription

	targeting := updateSegmentTargetingRequest{
		Comment:  "Terraform provider Phase 0 probe",
		Included: append([]string{}, names.includedUsers...),
		Excluded: append([]string{}, names.excludedUsers...),
		Rules: []segmentMatchRule{{
			ID:   names.ruleID,
			Name: names.ruleName,
			Conditions: []segmentCondition{{
				ID:       names.conditionID,
				Property: "keyId",
				Operator: "IsOneOf",
				Value:    names.conditionValue,
			}},
		}},
	}
	report.Workarounds = append(
		report.Workarounds,
		"segment_condition_operator_documentation_mapping",
	)
	result, requestErr = lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentTargetingPath(environmentID, segmentID),
		segmentTargetingTemplate,
		targeting,
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentTargetingTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterTargeting, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther = canonicalSegment(current)
	afterOther = canonicalSegment(afterTargeting)
	beforeOther.Included = []string{}
	beforeOther.Excluded = []string{}
	beforeOther.Rules = []segmentMatchRule{}
	afterOther.Included = []string{}
	afterOther.Excluded = []string{}
	afterOther.Rules = []segmentMatchRule{}
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if !reflect.DeepEqual(
		canonicalStringSet(afterTargeting.Included),
		canonicalStringSet(targeting.Included),
	) ||
		!reflect.DeepEqual(
			canonicalStringSet(afterTargeting.Excluded),
			canonicalStringSet(targeting.Excluded),
		) ||
		!reflect.DeepEqual(afterTargeting.Rules, targeting.Rules) ||
		!unrelatedPreserved {
		return errors.New(
			"segment targeting update did not preserve its canonical contract",
		)
	}
	report.Segment.TargetingUpdated = true
	current = afterTargeting

	result, requestErr = lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentTagsPath(environmentID, segmentID),
		segmentTagsTemplate,
		updateSegmentTagsRequest{
			Comment: "Terraform provider Phase 0 probe",
			ID:      segmentID,
			Tags:    append([]string{}, names.tags...),
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentTagsTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterTags, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther = canonicalSegment(current)
	afterOther = canonicalSegment(afterTags)
	beforeOther.Tags = []string{}
	afterOther.Tags = []string{}
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if !reflect.DeepEqual(
		canonicalStringSet(afterTags.Tags),
		canonicalStringSet(names.tags),
	) ||
		!unrelatedPreserved {
		return errors.New("segment tags update changed unrelated canonical fields")
	}
	report.Segment.TagsUpdated = true
	current = afterTags

	result, requestErr = lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentArchivePath(environmentID, segmentID),
		segmentArchiveTemplate,
		segmentResourceChangeRequest{
			Comment: "Terraform provider Phase 0 probe",
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentArchiveTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterArchive, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther = canonicalSegment(current)
	afterOther = canonicalSegment(afterArchive)
	beforeOther.IsArchived = false
	afterOther.IsArchived = false
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if !afterArchive.IsArchived || !unrelatedPreserved {
		return errors.New("segment archive changed unrelated canonical fields")
	}
	report.Segment.Archived = true
	current = afterArchive

	result, requestErr = lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPut,
		segmentRestorePath(environmentID, segmentID),
		segmentRestoreTemplate,
		segmentResourceChangeRequest{
			Comment: "Terraform provider Phase 0 probe",
		},
	)
	if err := requireLifecycleSuccess(
		http.MethodPut,
		segmentRestoreTemplate,
		result,
		requestErr,
	); err != nil {
		return err
	}
	afterRestore, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	beforeOther = canonicalSegment(current)
	afterOther = canonicalSegment(afterRestore)
	beforeOther.IsArchived = false
	afterOther.IsArchived = false
	unrelatedPreserved = unrelatedPreserved &&
		reflect.DeepEqual(beforeOther, afterOther)
	if afterRestore.IsArchived || !unrelatedPreserved {
		return errors.New("segment restore changed unrelated canonical fields")
	}
	report.Segment.Restored = true
	current = afterRestore

	referencesResult, referencesErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		segmentFlagReferencesPath(environmentID, segmentID),
		segmentFlagReferencesTemplate,
		nil,
	)
	if err := requireLifecycleSuccess(
		http.MethodGet,
		segmentFlagReferencesTemplate,
		referencesResult,
		referencesErr,
	); err != nil {
		return err
	}
	references, err := decodeLifecycleData[[]segmentFlagReference](
		referencesResult,
	)
	if err != nil {
		return err
	}
	report.Segment.FlagReferenceCount = len(references)
	report.Workarounds = append(
		report.Workarounds,
		"segment_delete_flag_reference_preflight",
	)
	if len(references) != 0 {
		return errors.New(
			"segment has feature-flag references; refusing Delete and preserving cleanup state",
		)
	}

	secondStableRead, err := lifecycleReadSegment(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	report.Segment.CanonicalReadsEqual = reflect.DeepEqual(
		canonicalSegment(current),
		canonicalSegment(secondStableRead),
	)
	report.Segment.UnrelatedFieldsPreserved = unrelatedPreserved
	if !report.Segment.CanonicalReadsEqual ||
		!report.Segment.UnrelatedFieldsPreserved {
		return errors.New("segment repeated canonical Reads did not converge")
	}
	recordSegmentShape(
		report.Segment,
		secondStableRead,
		names,
		environmentScope,
	)
	if !report.Segment.KeyPreserved ||
		!report.Segment.TypePreserved ||
		!report.Segment.ScopesPreserved ||
		!report.Segment.EnvironmentSpecific {
		return errors.New(
			"segment update sequence changed replace-only identity fields",
		)
	}

	presentByID, err := lifecycleFindSegmentByIDAnyState(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if err != nil {
		return err
	}
	report.Segment.PresentExactIDMatches = presentByID.Count
	if presentByID.Count != 1 {
		return errors.New(
			"all-page segment list did not contain exactly one owned ID before Delete",
		)
	}

	deleteResult, deleteErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodDelete,
		segmentPath(environmentID, segmentID),
		segmentItemTemplate,
		segmentResourceChangeRequest{
			Comment: "Terraform provider Phase 0 probe",
		},
	)
	report.Segment.InitialDelete = compatibilityOutcome(
		deleteResult,
		deleteErr,
		-1,
	)
	if observationHasErrorCode(
		deleteResult.Observation,
		"CannotDeleteUnArchivedSegment",
	) {
		archiveResult, archiveErr := lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodPut,
			segmentArchivePath(environmentID, segmentID),
			segmentArchiveTemplate,
			segmentResourceChangeRequest{
				Comment: "Terraform provider Phase 0 delete prerequisite",
			},
		)
		archiveOutcome := compatibilityOutcome(archiveResult, archiveErr, 1)
		report.Segment.DeleteArchive = &archiveOutcome
		if err := requireLifecycleSuccess(
			http.MethodPut,
			segmentArchiveTemplate,
			archiveResult,
			archiveErr,
		); err != nil {
			return err
		}
		report.Segment.ArchivedBeforeDelete = true
		report.Workarounds = append(
			report.Workarounds,
			"segment_archive_before_delete",
		)
		deleteResult, deleteErr = lifecycleRequest(
			ctx,
			client,
			report,
			http.MethodDelete,
			segmentPath(environmentID, segmentID),
			segmentItemTemplate,
			segmentResourceChangeRequest{
				Comment: "Terraform provider Phase 0 probe",
			},
		)
	}
	postDeleteResult, postDeleteErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		segmentPath(environmentID, segmentID),
		segmentItemTemplate,
		nil,
	)
	report.Segment.PostDeleteRead = compatibilityOutcome(
		postDeleteResult,
		postDeleteErr,
		-1,
	)
	if report.Segment.PostDeleteRead.Classification != ClassificationSuccess {
		report.Workarounds = append(
			report.Workarounds,
			"segment_post_delete_all_page_exact_id_fallback",
		)
	}

	afterDeleteID, idLookupErr := lifecycleFindSegmentByIDAnyState(
		ctx,
		client,
		report,
		environmentID,
		segmentID,
	)
	if idLookupErr != nil {
		return errors.New("segment post-delete all-page exact-ID fallback failed")
	}
	afterDeleteKey, keyLookupErr := lifecycleFindSegmentByKeyAnyState(
		ctx,
		client,
		report,
		environmentID,
		names.key,
	)
	if keyLookupErr != nil {
		return errors.New("segment post-delete all-page exact-key fallback failed")
	}
	report.Segment.AbsentExactIDMatches = afterDeleteID.Count
	report.Segment.AbsentExactKeyMatches = afterDeleteKey.Count
	report.Segment.PostDeleteRead.ExactMatchCount = afterDeleteID.Count
	if report.Segment.PostDeleteRead.Classification == ClassificationSuccess {
		return errors.New(
			"segment direct Read remained present after Delete; preserving cleanup state",
		)
	}
	if afterDeleteID.Count != 0 || afterDeleteKey.Count != 0 {
		if deleteErr != nil {
			return deleteErr
		}
		return errors.New(
			"segment Delete returned but exact identity remains; parent cleanup is pending",
		)
	}
	if err := requireDeleteOrVerifiedAbsence(
		http.MethodDelete,
		segmentItemTemplate,
		deleteResult,
		deleteErr,
		report,
		"segment_delete_verified_by_all_page_exact_absence",
	); err != nil {
		return err
	}
	segmentIdentity := ResourceIdentity{
		ID:            segmentID,
		Key:           names.key,
		ProjectID:     projectID,
		EnvironmentID: environmentID,
	}
	if err := inventory.MarkCleaned(
		cfg.Target,
		ResourceSegment,
		segmentIdentity,
	); err != nil {
		return err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return err
	}
	report.Cleanup.SegmentAbsent = true
	report.Cleanup.PendingInventory = inventory.Pending()
	return nil
}

func lifecycleFindSegmentByKey(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedKey string,
) (ExactMatch[segmentListItem], error) {
	return lifecycleFindSegment(
		ctx,
		client,
		report,
		environmentID,
		func(item segmentListItem) string { return item.Key },
		expectedKey,
	)
}

func lifecycleFindSegmentByID(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedID string,
) (ExactMatch[segmentListItem], error) {
	return lifecycleFindSegment(
		ctx,
		client,
		report,
		environmentID,
		func(item segmentListItem) string { return item.ID },
		expectedID,
	)
}

func lifecycleFindSegmentByKeyAnyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedKey string,
) (ExactMatch[segmentListItem], error) {
	return lifecycleFindSegmentAnyState(
		ctx,
		client,
		report,
		environmentID,
		func(item segmentListItem) string { return item.Key },
		expectedKey,
	)
}

func lifecycleFindSegmentByIDAnyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	expectedID string,
) (ExactMatch[segmentListItem], error) {
	return lifecycleFindSegmentAnyState(
		ctx,
		client,
		report,
		environmentID,
		func(item segmentListItem) string { return item.ID },
		expectedID,
	)
}

func lifecycleFindSegmentAnyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	identity func(segmentListItem) string,
	expected string,
) (ExactMatch[segmentListItem], error) {
	active := false
	activeMatch, err := lifecycleFindSegmentState(
		ctx,
		client,
		report,
		environmentID,
		identity,
		expected,
		&active,
	)
	if err != nil {
		return ExactMatch[segmentListItem]{}, err
	}
	archived := true
	archivedMatch, err := lifecycleFindSegmentState(
		ctx,
		client,
		report,
		environmentID,
		identity,
		expected,
		&archived,
	)
	if err != nil {
		return ExactMatch[segmentListItem]{}, err
	}
	result := ExactMatch[segmentListItem]{
		Count: activeMatch.Count + archivedMatch.Count,
		Items: append(
			append([]segmentListItem{}, activeMatch.Items...),
			archivedMatch.Items...,
		),
	}
	if result.Count > 0 {
		result.Item = result.Items[0]
	}
	return result, nil
}

func lifecycleFindSegment(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	identity func(segmentListItem) string,
	expected string,
) (ExactMatch[segmentListItem], error) {
	return lifecycleFindSegmentState(
		ctx,
		client,
		report,
		environmentID,
		identity,
		expected,
		nil,
	)
}

func lifecycleFindSegmentState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	identity func(segmentListItem) string,
	expected string,
	archived *bool,
) (ExactMatch[segmentListItem], error) {
	return FindExactAcrossAllPages(
		ctx,
		segmentPageSize,
		func(
			ctx context.Context,
			pageIndex int,
			pageSize int,
		) (Page[segmentListItem], error) {
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
				segmentCollectionPath(environmentID)+"?"+query.Encode(),
				segmentCollectionTemplate,
				nil,
			)
			if err := requireLifecycleSuccess(
				http.MethodGet,
				segmentCollectionTemplate,
				result,
				requestErr,
			); err != nil {
				return Page[segmentListItem]{}, err
			}
			page, err := decodeLifecycleData[segmentPage](result)
			if err != nil {
				return Page[segmentListItem]{}, err
			}
			return Page[segmentListItem]{
				Items:      page.Items,
				TotalCount: page.TotalCount,
			}, nil
		},
		identity,
		expected,
		nil,
	)
}

func lifecycleResolveSegmentEnvironmentScope(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	projectKey string,
	environmentKey string,
) (string, error) {
	items, err := lifecycleListSegmentsAnyState(
		ctx,
		client,
		report,
		environmentID,
	)
	if err != nil {
		return "", errors.New(
			"segment scope discovery could not list documented segment views",
		)
	}
	organizationPrefixes := map[string]struct{}{}
	seenIDs := map[string]struct{}{}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, seen := seenIDs[item.ID]; seen {
			continue
		}
		seenIDs[item.ID] = struct{}{}
		segment, readErr := lifecycleReadSegment(
			ctx,
			client,
			report,
			environmentID,
			item.ID,
		)
		if readErr != nil {
			return "", errors.New(
				"segment scope discovery could not read an exact documented segment identity",
			)
		}
		for _, scope := range segment.Scopes {
			prefix, ok := segmentOrganizationScopePrefix(scope)
			if ok {
				organizationPrefixes[prefix] = struct{}{}
			}
		}
	}
	if len(organizationPrefixes) != 1 {
		return "", errors.New(
			"segment scope discovery did not produce one unambiguous organization prefix",
		)
	}
	var organizationPrefix string
	for prefix := range organizationPrefixes {
		organizationPrefix = prefix
	}
	report.Workarounds = append(
		report.Workarounds,
		"segment_scope_resource_name_discovered_from_exact_public_reads",
	)
	return organizationPrefix +
		":project/" + projectKey +
		":env/" + environmentKey, nil
}

func lifecycleListSegmentsAnyState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
) ([]segmentListItem, error) {
	active := false
	activeItems, err := lifecycleListSegmentsState(
		ctx,
		client,
		report,
		environmentID,
		&active,
	)
	if err != nil {
		return nil, err
	}
	archived := true
	archivedItems, err := lifecycleListSegmentsState(
		ctx,
		client,
		report,
		environmentID,
		&archived,
	)
	if err != nil {
		return nil, err
	}
	return append(activeItems, archivedItems...), nil
}

func lifecycleListSegmentsState(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	archived *bool,
) ([]segmentListItem, error) {
	return CollectAllPages(
		ctx,
		segmentPageSize,
		func(
			ctx context.Context,
			pageIndex int,
			pageSize int,
		) (Page[segmentListItem], error) {
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
				segmentCollectionPath(environmentID)+"?"+query.Encode(),
				segmentCollectionTemplate,
				nil,
			)
			if err := requireLifecycleSuccess(
				http.MethodGet,
				segmentCollectionTemplate,
				result,
				requestErr,
			); err != nil {
				return Page[segmentListItem]{}, err
			}
			page, err := decodeLifecycleData[segmentPage](result)
			if err != nil {
				return Page[segmentListItem]{}, err
			}
			return Page[segmentListItem]{
				Items:      page.Items,
				TotalCount: page.TotalCount,
			}, nil
		},
	)
}

func segmentOrganizationScopePrefix(scope string) (string, bool) {
	const prefix = "organization/"
	head, _, _ := strings.Cut(scope, ":")
	if !strings.HasPrefix(head, prefix) {
		return "", false
	}
	key := strings.TrimPrefix(head, prefix)
	if key == "" ||
		strings.ContainsAny(key, "/*: \t\r\n") {
		return "", false
	}
	return head, true
}

func lifecycleReadSegment(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	environmentID string,
	segmentID string,
) (segmentWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		segmentPath(environmentID, segmentID),
		segmentItemTemplate,
		nil,
	)
	if err := requireLifecycleSuccess(
		http.MethodGet,
		segmentItemTemplate,
		result,
		requestErr,
	); err != nil {
		return segmentWire{}, err
	}
	return decodeLifecycleData[segmentWire](result)
}

func segmentNamesForPrefix(prefix string) segmentNames {
	return segmentNames{
		name:               prefix + " segment",
		updatedName:        prefix + " segment updated",
		key:                prefix,
		description:        "Terraform provider Phase 0 probe",
		updatedDescription: "Terraform provider Phase 0 probe updated",
		includedUsers: []string{
			prefix + "-included-b",
			prefix + "-included-a",
		},
		excludedUsers: []string{prefix + "-excluded"},
		tags: []string{
			"terraform-phase0",
			"segment-probe",
		},
		ruleID:         deterministicProbeUUID(prefix + ":segment:rule"),
		ruleName:       "Terraform Phase 0 rule",
		conditionID:    deterministicProbeUUID(prefix + ":segment:condition"),
		conditionValue: fmt.Sprintf("[\"%s-rule-user\"]", prefix),
	}
}

func canonicalSegment(input segmentWire) segmentWire {
	output := input
	output.CreatedAt = ""
	output.UpdatedAt = ""
	output.Scopes = canonicalStringSet(input.Scopes)
	output.Included = canonicalStringSet(input.Included)
	output.Excluded = canonicalStringSet(input.Excluded)
	output.Tags = canonicalStringSet(input.Tags)
	output.Rules = make([]segmentMatchRule, len(input.Rules))
	for index, rule := range input.Rules {
		output.Rules[index] = rule
		output.Rules[index].Conditions = append(
			[]segmentCondition{},
			rule.Conditions...,
		)
	}
	return output
}

func recordSegmentShape(
	summary *SegmentLifecycleSummary,
	segment segmentWire,
	names segmentNames,
	environmentScope string,
) {
	summary.KeyPreserved = segment.Key == names.key
	summary.TypePreserved = segment.Type == "environment-specific"
	summary.ScopesPreserved = reflect.DeepEqual(
		canonicalStringSet(segment.Scopes),
		[]string{environmentScope},
	)
	summary.EnvironmentSpecific = segment.IsEnvironmentSpecific
	summary.IncludedCount = len(canonicalStringSet(segment.Included))
	summary.ExcludedCount = len(canonicalStringSet(segment.Excluded))
	summary.RuleCount = len(segment.Rules)
	for _, rule := range segment.Rules {
		summary.ConditionCount += len(rule.Conditions)
	}
	summary.TagCount = len(canonicalStringSet(segment.Tags))
}

func segmentCollectionPath(environmentID string) string {
	return "/api/v1/envs/" + url.PathEscape(environmentID) + "/segments"
}

func segmentPath(environmentID string, segmentID string) string {
	return segmentCollectionPath(environmentID) + "/" + url.PathEscape(segmentID)
}

func segmentNamePath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/name"
}

func segmentDescriptionPath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/description"
}

func segmentTargetingPath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/targeting"
}

func segmentTagsPath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/tags"
}

func segmentArchivePath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/archive"
}

func segmentRestorePath(environmentID string, segmentID string) string {
	return segmentPath(environmentID, segmentID) + "/restore"
}

func segmentFlagReferencesPath(
	environmentID string,
	segmentID string,
) string {
	return segmentPath(environmentID, segmentID) + "/flag-references"
}
