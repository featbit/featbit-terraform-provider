package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

const (
	projectCollectionPath = "/api/v1/projects"
	projectItemTemplate   = "/api/v1/projects/{id}"
	environmentCollection = "/api/v1/projects/{projectId}/envs"
	environmentItem       = "/api/v1/projects/{projectId}/envs/{id}"
)

// ProjectEnvironmentLifecycleReport contains only sanitized observations and
// normalized behavior. Concrete resource IDs never appear in this value.
type ProjectEnvironmentLifecycleReport struct {
	Target        Target                         `json:"target"`
	MutationScope string                         `json:"mutation_scope"`
	Observations  []Observation                  `json:"observations"`
	Workarounds   []string                       `json:"workarounds,omitempty"`
	Compatibility *LifecycleCompatibilitySummary `json:"compatibility,omitempty"`
	ChildReads    *ChildCollectionReadSummary    `json:"child_reads,omitempty"`
	Project       ProjectLifecycleSummary        `json:"project"`
	Environment   EnvironmentLifecycleSummary    `json:"environment"`
	FeatureFlag   *FeatureFlagLifecycleSummary   `json:"feature_flag,omitempty"`
	FeatureFlags  []FeatureFlagLifecycleSummary  `json:"feature_flag_types,omitempty"`
	Segment       *SegmentLifecycleSummary       `json:"segment,omitempty"`
	Cleanup       LifecycleCleanupSummary        `json:"cleanup"`
}

type LifecycleCompatibilitySummary struct {
	ProjectValidation              CompatibilityOutcome `json:"project_validation"`
	ProjectDuplicate               CompatibilityOutcome `json:"project_duplicate"`
	ProjectPresentExactMatches     int                  `json:"project_present_exact_matches"`
	ProjectPostDeleteRead          CompatibilityOutcome `json:"project_post_delete_read"`
	ProjectAbsentExactMatches      int                  `json:"project_absent_exact_matches"`
	EnvironmentValidation          CompatibilityOutcome `json:"environment_validation"`
	EnvironmentDuplicate           CompatibilityOutcome `json:"environment_duplicate"`
	EnvironmentPresentExactMatches int                  `json:"environment_present_exact_matches"`
	EnvironmentPostDeleteRead      CompatibilityOutcome `json:"environment_post_delete_read"`
	EnvironmentAbsentExactMatches  int                  `json:"environment_absent_exact_matches"`
}

type CompatibilityOutcome struct {
	HTTPStatus      int            `json:"http_status"`
	EnvelopeSuccess *bool          `json:"envelope_success"`
	Classification  Classification `json:"classification"`
	ErrorCodes      []string       `json:"error_codes"`
	ExactMatchCount int            `json:"exact_match_count"`
}

type lifecycleOptions struct {
	compatibilityChecks            bool
	featureFlagChecks              bool
	featureFlagCompatibilityChecks bool
	featureFlagVariationTypes      []string
	segmentChecks                  bool
	segmentCompatibilityChecks     bool
	childReadChecks                bool
}

type ProjectLifecycleSummary struct {
	Created              bool                      `json:"created"`
	Updated              bool                      `json:"updated"`
	KeyPreserved         bool                      `json:"key_preserved"`
	CanonicalReadsEqual  bool                      `json:"canonical_reads_equal"`
	AutoEnvironmentCount int                       `json:"auto_environment_count"`
	AutoEnvironments     []EnvironmentShapeSummary `json:"auto_environments"`
}

type EnvironmentLifecycleSummary struct {
	Created             bool                  `json:"created"`
	Updated             bool                  `json:"updated"`
	KeyPreserved        bool                  `json:"key_preserved"`
	CanonicalReadsEqual bool                  `json:"canonical_reads_equal"`
	SecretMetadata      SecretMetadataSummary `json:"secret_metadata"`
}

type LifecycleCleanupSummary struct {
	FeatureFlagAbsent bool `json:"feature_flag_absent,omitempty"`
	SegmentAbsent     bool `json:"segment_absent,omitempty"`
	EnvironmentAbsent bool `json:"environment_absent"`
	ProjectAbsent     bool `json:"project_absent"`
	PendingInventory  int  `json:"pending_inventory"`
}

type EnvironmentShapeSummary struct {
	Position             int                   `json:"position"`
	Name                 string                `json:"name"`
	Key                  string                `json:"key"`
	DescriptionPresent   bool                  `json:"description_present"`
	SettingsPresent      bool                  `json:"settings_present"`
	RequireChangeComment bool                  `json:"require_change_comment"`
	SecretMetadata       SecretMetadataSummary `json:"secret_metadata"`
}

type SecretMetadataSummary struct {
	Count             int      `json:"count"`
	IDFieldPresent    int      `json:"id_field_present"`
	NameFieldPresent  int      `json:"name_field_present"`
	TypeFieldPresent  int      `json:"type_field_present"`
	ValueFieldPresent int      `json:"value_field_present"`
	TypeClasses       []string `json:"type_classes"`
	ValueJSONTypes    []string `json:"value_json_types"`
}

type lifecycleNames struct {
	projectName        string
	projectUpdatedName string
	projectKey         string
	environmentName    string
	environmentUpdated string
	environmentKey     string
	description        string
	updatedDescription string
}

type projectWire struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Key          string            `json:"key"`
	Environments []environmentWire `json:"environments"`
}

type environmentWire struct {
	ID          string                   `json:"id"`
	ProjectID   string                   `json:"projectId"`
	Name        string                   `json:"name"`
	Key         string                   `json:"key"`
	Description string                   `json:"description"`
	Secrets     []secretMetadataWire     `json:"secrets"`
	Settings    *environmentSettingsWire `json:"settings"`
}

type environmentSettingsWire struct {
	RequireChangeComment bool `json:"requireChangeComment"`
}

type secretMetadataWire struct {
	IDPresent     bool
	NamePresent   bool
	TypePresent   bool
	ValuePresent  bool
	TypeClass     string
	ValueJSONType string
}

type createProjectRequest struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type updateProjectRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createEnvironmentRequest struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
}

type updateEnvironmentRequest struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Settings    *environmentSettingsWire `json:"settings,omitempty"`
}

type canonicalProject struct {
	ID           string
	Name         string
	Key          string
	Environments []canonicalEnvironment
}

type canonicalEnvironment struct {
	ID          string
	ProjectID   string
	Name        string
	Key         string
	Description string
	Settings    *environmentSettingsWire
	Secrets     SecretMetadataSummary
}

func (s *secretMetadataWire) UnmarshalJSON(content []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		return errors.New("decode secret metadata object")
	}
	if raw, ok := fields["id"]; ok {
		s.IDPresent = true
		_ = raw
	}
	if raw, ok := fields["name"]; ok {
		s.NamePresent = true
		_ = raw
	}
	if raw, ok := fields["type"]; ok {
		s.TypePresent = true
		var value string
		if json.Unmarshal(raw, &value) == nil {
			switch strings.ToLower(value) {
			case "server":
				s.TypeClass = "Server"
			case "client":
				s.TypeClass = "Client"
			default:
				s.TypeClass = "<OTHER>"
			}
		} else {
			s.TypeClass = jsonValueType(raw)
		}
	}
	if raw, ok := fields["value"]; ok {
		s.ValuePresent = true
		s.ValueJSONType = jsonValueType(raw)
	}
	return nil
}

// RunProjectEnvironmentLifecycle performs one isolated lifecycle. It accepts no
// project or environment IDs from the caller: every mutable ID must originate
// from this invocation's create response or its zero-before-create exact-key
// reconciliation.
func RunProjectEnvironmentLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{},
	)
}

func RunProjectEnvironmentCompatibilityLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{compatibilityChecks: true},
	)
}

// RunFeatureFlagLifecycle creates its own project and environment parents,
// probes only a flag under that newly created environment, and then uses the
// normal parent cleanup path. It accepts no remote IDs from the caller.
func RunFeatureFlagLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{
			featureFlagChecks:              true,
			featureFlagCompatibilityChecks: true,
		},
	)
}

// RunFeatureFlagCRUDLifecycle creates exactly one flag and exercises only its
// normal Create, Read, narrow name Update, and Delete path. It deliberately
// skips duplicate-create and stale-revision compatibility writes.
func RunFeatureFlagCRUDLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{featureFlagChecks: true},
	)
}

// RunFeatureFlagTypeMatrixLifecycle creates one owned project/environment and
// sequentially exercises String, Number, and JSON flags. Every flag is removed
// and proven absent before the next type starts. Boolean is covered by the
// separate one-flag lifecycle.
func RunFeatureFlagTypeMatrixLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{
			featureFlagVariationTypes: []string{"string", "number", "json"},
		},
	)
}

// RunSegmentLifecycle creates its own project and environment parents, probes
// only an environment-specific segment under that newly created environment,
// and then uses the normal parent cleanup path. It accepts no remote IDs from
// the caller and deliberately excludes workspace-wide shared segments.
func RunSegmentLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{
			segmentChecks:              true,
			segmentCompatibilityChecks: true,
		},
	)
}

// RunSegmentCRUDLifecycle creates exactly one environment-specific segment and
// exercises its documented narrow updates, archive/restore, reference preflight,
// and Delete path. It deliberately skips the duplicate-create compatibility
// write so an approval scoped to one child object remains one child object.
func RunSegmentCRUDLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{segmentChecks: true},
	)
}

// RunChildCollectionReadLifecycle creates its own project and environment,
// performs only exact read/absence checks for flag and segment collections in
// that owned environment, and then removes the parents. It never mutates a
// flag or segment.
func RunChildCollectionReadLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
) (ProjectEnvironmentLifecycleReport, error) {
	return runProjectEnvironmentLifecycle(
		ctx,
		cfg,
		client,
		inventoryPath,
		lifecycleOptions{childReadChecks: true},
	)
}

func runProjectEnvironmentLifecycle(
	ctx context.Context,
	cfg Config,
	client *Client,
	inventoryPath string,
	options lifecycleOptions,
) (ProjectEnvironmentLifecycleReport, error) {
	report := ProjectEnvironmentLifecycleReport{
		Target:        cfg.Target,
		MutationScope: "newly-created-prefix-only",
	}
	if options.compatibilityChecks {
		report.Compatibility = &LifecycleCompatibilitySummary{}
	}
	if options.featureFlagChecks {
		report.FeatureFlag = &FeatureFlagLifecycleSummary{}
	}
	if len(options.featureFlagVariationTypes) > 0 {
		report.FeatureFlags = make(
			[]FeatureFlagLifecycleSummary,
			0,
			len(options.featureFlagVariationTypes),
		)
	}
	if options.segmentChecks {
		report.Segment = &SegmentLifecycleSummary{}
	}
	if options.childReadChecks {
		report.ChildReads = &ChildCollectionReadSummary{}
	}
	if err := cfg.ValidateMutation(); err != nil {
		return report, err
	}
	if client == nil {
		return report, errors.New("lifecycle client is required")
	}
	if inventoryPath == "" {
		return report, errors.New("cleanup inventory path is required")
	}

	inventory, err := LoadInventory(inventoryPath)
	if err != nil {
		return report, err
	}
	if inventory.Pending() != 0 {
		report.Cleanup.PendingInventory = inventory.Pending()
		return report, errors.New("cleanup inventory has pending resources; resolve them before a new lifecycle")
	}

	names := namesForPrefix(cfg.ResourcePrefix)
	projects, err := lifecycleListProjects(ctx, client, &report)
	if err != nil {
		return report, err
	}
	if countProjectsByKey(projects, names.projectKey) != 0 {
		return report, errors.New("exact project key already exists; refusing to mutate a pre-existing object")
	}

	var project projectWire
	if options.compatibilityChecks {
		project, err = lifecycleCreateProjectWithValidation(
			ctx,
			client,
			&report,
			names,
		)
	} else {
		project, err = lifecycleCreateProject(ctx, client, &report, names)
	}
	if err != nil {
		return report, err
	}
	projectIdentity := ResourceIdentity{ID: project.ID, Key: names.projectKey}
	if err := inventory.Track(InventoryEntry{
		Target:   cfg.Target,
		Type:     ResourceProject,
		Identity: projectIdentity,
		TODO:     "P0-040",
	}); err != nil {
		return report, err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return report, err
	}
	report.Project.Created = true
	report.Cleanup.PendingInventory = inventory.Pending()

	projectAfterCreate, err := lifecycleReadProject(ctx, client, &report, project.ID)
	if err != nil {
		return report, err
	}
	if projectAfterCreate.ID != project.ID || projectAfterCreate.Key != names.projectKey {
		return report, errors.New("created project canonical read did not preserve exact identity")
	}
	report.Project.AutoEnvironmentCount = len(projectAfterCreate.Environments)
	report.Project.AutoEnvironments = summarizeEnvironmentShapes(projectAfterCreate.Environments)

	if options.compatibilityChecks {
		if err := lifecycleProbeDuplicateProject(
			ctx,
			cfg,
			client,
			&report,
			&inventory,
			inventoryPath,
			project,
			names,
		); err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
		projectsWithCreated, listErr := lifecycleListProjects(ctx, client, &report)
		if listErr != nil {
			return report, listErr
		}
		report.Compatibility.ProjectPresentExactMatches = countProjectsByID(
			projectsWithCreated,
			project.ID,
		)
		if report.Compatibility.ProjectPresentExactMatches != 1 {
			return report, errors.New(
				"complete project collection did not contain exactly one created project",
			)
		}
	}

	result, requestErr := lifecycleRequest(
		ctx,
		client,
		&report,
		http.MethodPut,
		projectPath(project.ID),
		projectItemTemplate,
		updateProjectRequest{ID: project.ID, Name: names.projectUpdatedName},
	)
	if err := requireLifecycleSuccess(http.MethodPut, projectItemTemplate, result, requestErr); err != nil {
		return report, err
	}
	report.Project.Updated = true

	firstProjectRead, err := lifecycleReadProject(ctx, client, &report, project.ID)
	if err != nil {
		return report, err
	}
	secondProjectRead, err := lifecycleReadProject(ctx, client, &report, project.ID)
	if err != nil {
		return report, err
	}
	report.Project.CanonicalReadsEqual = reflect.DeepEqual(
		canonicalizeProject(firstProjectRead),
		canonicalizeProject(secondProjectRead),
	)
	report.Project.KeyPreserved = secondProjectRead.Key == names.projectKey
	if secondProjectRead.Name != names.projectUpdatedName ||
		!report.Project.KeyPreserved ||
		!report.Project.CanonicalReadsEqual {
		return report, errors.New("project update did not converge to a stable canonical read")
	}

	if countEnvironmentsByKey(secondProjectRead.Environments, names.environmentKey) != 0 {
		return report, errors.New("exact environment key already exists in the newly created project")
	}

	var environment environmentWire
	if options.compatibilityChecks {
		environment, err = lifecycleCreateEnvironmentWithValidation(
			ctx,
			client,
			&report,
			project.ID,
			names,
		)
	} else {
		environment, err = lifecycleCreateEnvironment(
			ctx,
			client,
			&report,
			project.ID,
			names,
		)
	}
	if err != nil {
		return report, err
	}
	environmentIdentity := ResourceIdentity{
		ID:        environment.ID,
		ProjectID: project.ID,
		Key:       names.environmentKey,
	}
	if err := inventory.Track(InventoryEntry{
		Target:   cfg.Target,
		Type:     ResourceEnvironment,
		Identity: environmentIdentity,
		TODO:     "P0-043",
	}); err != nil {
		return report, err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return report, err
	}
	report.Environment.Created = true
	report.Cleanup.PendingInventory = inventory.Pending()

	projectAfterEnvironmentCreate, err := lifecycleReadProject(ctx, client, &report, project.ID)
	if err != nil {
		return report, err
	}
	ownedEnvironment, ownedEnvironmentCount := environmentByID(
		projectAfterEnvironmentCreate.Environments,
		environment.ID,
	)
	if ownedEnvironmentCount != 1 || ownedEnvironment.Key != names.environmentKey {
		return report, errors.New(
			"created environment was not confirmed under the newly created project; refusing update",
		)
	}
	if options.compatibilityChecks {
		report.Compatibility.EnvironmentPresentExactMatches = ownedEnvironmentCount
		if err := lifecycleProbeDuplicateEnvironment(
			ctx,
			cfg,
			client,
			&report,
			&inventory,
			inventoryPath,
			project.ID,
			environment,
			names,
		); err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
	}

	environmentAfterCreate, err := lifecycleReadEnvironment(
		ctx,
		client,
		&report,
		project.ID,
		environment.ID,
	)
	if err != nil {
		return report, err
	}

	result, requestErr = lifecycleRequest(
		ctx,
		client,
		&report,
		http.MethodPut,
		environmentPath(project.ID, environment.ID),
		environmentItem,
		updateEnvironmentRequest{
			ID:          environment.ID,
			Name:        names.environmentUpdated,
			Description: names.updatedDescription,
			Settings:    environmentAfterCreate.Settings,
		},
	)
	if err := requireLifecycleSuccess(http.MethodPut, environmentItem, result, requestErr); err != nil {
		return report, err
	}
	report.Environment.Updated = true

	firstEnvironmentRead, err := lifecycleReadEnvironment(
		ctx,
		client,
		&report,
		project.ID,
		environment.ID,
	)
	if err != nil {
		return report, err
	}
	secondEnvironmentRead, err := lifecycleReadEnvironment(
		ctx,
		client,
		&report,
		project.ID,
		environment.ID,
	)
	if err != nil {
		return report, err
	}
	report.Environment.CanonicalReadsEqual = reflect.DeepEqual(
		canonicalizeEnvironment(firstEnvironmentRead),
		canonicalizeEnvironment(secondEnvironmentRead),
	)
	report.Environment.SecretMetadata = summarizeSecrets(secondEnvironmentRead.Secrets)
	if secondEnvironmentRead.Name != names.environmentUpdated ||
		secondEnvironmentRead.Description != names.updatedDescription ||
		!report.Environment.CanonicalReadsEqual {
		return report, errors.New("environment update did not converge to a stable canonical read")
	}

	projectWithEnvironment, err := lifecycleReadProject(ctx, client, &report, project.ID)
	if err != nil {
		return report, err
	}
	exactEnvironment, exactCount := environmentByID(projectWithEnvironment.Environments, environment.ID)
	report.Environment.KeyPreserved = exactCount == 1 && exactEnvironment.Key == names.environmentKey
	if !report.Environment.KeyPreserved {
		return report, errors.New("environment exact identity or key was not preserved")
	}

	if options.featureFlagChecks {
		if err := lifecycleProbeFeatureFlag(
			ctx,
			cfg,
			client,
			&report,
			&inventory,
			inventoryPath,
			project.ID,
			environment.ID,
			names.projectKey,
			"boolean",
			"P0-050",
			options.featureFlagCompatibilityChecks,
		); err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
	}
	for _, variationType := range options.featureFlagVariationTypes {
		report.FeatureFlag = &FeatureFlagLifecycleSummary{}
		err := lifecycleProbeFeatureFlag(
			ctx,
			cfg,
			client,
			&report,
			&inventory,
			inventoryPath,
			project.ID,
			environment.ID,
			names.projectKey,
			variationType,
			"P0-051",
			false,
		)
		report.FeatureFlags = append(
			report.FeatureFlags,
			*report.FeatureFlag,
		)
		report.FeatureFlag = nil
		if err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
	}
	if options.segmentChecks {
		if err := lifecycleProbeSegment(
			ctx,
			cfg,
			client,
			&report,
			&inventory,
			inventoryPath,
			project.ID,
			environment.ID,
			names.projectKey,
			names.environmentKey,
			options.segmentCompatibilityChecks,
		); err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
	}
	if options.childReadChecks {
		if err := lifecycleProbeChildCollections(
			ctx,
			client,
			&report,
			environment.ID,
			names.projectKey,
		); err != nil {
			report.Cleanup.PendingInventory = inventory.Pending()
			return report, err
		}
	}

	deleteEnvironmentResult, deleteEnvironmentErr := lifecycleRequest(
		ctx,
		client,
		&report,
		http.MethodDelete,
		environmentPath(project.ID, environment.ID),
		environmentItem,
		nil,
	)
	if options.compatibilityChecks {
		postDeleteResult, postDeleteErr := lifecycleRequest(
			ctx,
			client,
			&report,
			http.MethodGet,
			environmentPath(project.ID, environment.ID),
			environmentItem,
			nil,
		)
		report.Compatibility.EnvironmentPostDeleteRead = compatibilityOutcome(
			postDeleteResult,
			postDeleteErr,
			0,
		)
		if report.Compatibility.EnvironmentPostDeleteRead.Classification !=
			ClassificationSuccess {
			report.Workarounds = append(
				report.Workarounds,
				"environment_post_delete_exact_parent_fallback",
			)
		}
	}
	projectAfterEnvironmentDelete, verifyEnvironmentErr := lifecycleReadProject(
		ctx,
		client,
		&report,
		project.ID,
	)
	if verifyEnvironmentErr != nil {
		return report, verifyEnvironmentErr
	}
	_, remainingEnvironmentCount := environmentByID(
		projectAfterEnvironmentDelete.Environments,
		environment.ID,
	)
	if options.compatibilityChecks {
		report.Compatibility.EnvironmentAbsentExactMatches = remainingEnvironmentCount
		report.Compatibility.EnvironmentPostDeleteRead.ExactMatchCount =
			remainingEnvironmentCount
	}
	if remainingEnvironmentCount != 0 {
		if deleteEnvironmentErr != nil {
			return report, deleteEnvironmentErr
		}
		return report, fmt.Errorf(
			"environment delete classification %s but exact identity remains",
			Classify(deleteEnvironmentResult.Observation, nil),
		)
	}
	if err := requireDeleteOrVerifiedAbsence(
		http.MethodDelete,
		environmentItem,
		deleteEnvironmentResult,
		deleteEnvironmentErr,
		&report,
		"environment_delete_verified_by_exact_absence",
	); err != nil {
		return report, err
	}
	if err := inventory.MarkCleaned(cfg.Target, ResourceEnvironment, environmentIdentity); err != nil {
		return report, err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return report, err
	}
	report.Cleanup.EnvironmentAbsent = true
	report.Cleanup.PendingInventory = inventory.Pending()

	deleteProjectResult, deleteProjectErr := lifecycleRequest(
		ctx,
		client,
		&report,
		http.MethodDelete,
		projectPath(project.ID),
		projectItemTemplate,
		nil,
	)
	if options.compatibilityChecks {
		postDeleteResult, postDeleteErr := lifecycleRequest(
			ctx,
			client,
			&report,
			http.MethodGet,
			projectPath(project.ID),
			projectItemTemplate,
			nil,
		)
		report.Compatibility.ProjectPostDeleteRead = compatibilityOutcome(
			postDeleteResult,
			postDeleteErr,
			0,
		)
		if report.Compatibility.ProjectPostDeleteRead.Classification !=
			ClassificationSuccess {
			report.Workarounds = append(
				report.Workarounds,
				"project_post_delete_exact_collection_fallback",
			)
		}
	}
	projectsAfterDelete, verifyProjectErr := lifecycleListProjects(ctx, client, &report)
	if verifyProjectErr != nil {
		return report, verifyProjectErr
	}
	projectAbsentMatches := countProjectsByID(projectsAfterDelete, project.ID)
	if options.compatibilityChecks {
		report.Compatibility.ProjectAbsentExactMatches = projectAbsentMatches
		report.Compatibility.ProjectPostDeleteRead.ExactMatchCount =
			projectAbsentMatches
	}
	if projectAbsentMatches != 0 {
		if deleteProjectErr != nil {
			return report, deleteProjectErr
		}
		return report, fmt.Errorf(
			"project delete classification %s but exact identity remains",
			Classify(deleteProjectResult.Observation, nil),
		)
	}
	if err := requireDeleteOrVerifiedAbsence(
		http.MethodDelete,
		projectItemTemplate,
		deleteProjectResult,
		deleteProjectErr,
		&report,
		"project_delete_verified_by_exact_absence",
	); err != nil {
		return report, err
	}
	if err := inventory.MarkCleaned(cfg.Target, ResourceProject, projectIdentity); err != nil {
		return report, err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return report, err
	}
	report.Cleanup.ProjectAbsent = true
	report.Cleanup.PendingInventory = inventory.Pending()
	return report, nil
}

func MarshalProjectEnvironmentLifecycleReport(report ProjectEnvironmentLifecycleReport) ([]byte, error) {
	content, err := json.Marshal(report)
	if err != nil {
		return nil, errors.New("encode lifecycle report")
	}
	return append(RedactJSON(content), '\n'), nil
}

func lifecycleCreateProjectWithValidation(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	names lifecycleNames,
) (projectWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		projectCollectionPath,
		projectCollectionPath,
		createProjectRequest{Name: "", Key: names.projectKey},
	)
	outcome := compatibilityOutcome(result, requestErr, -1)

	projects, listErr := lifecycleListProjects(ctx, client, report)
	if listErr == nil {
		project, count := projectByKey(projects, names.projectKey)
		outcome.ExactMatchCount = count
		report.Compatibility.ProjectValidation = outcome
		switch {
		case count == 1 && trackableProject(project):
			report.Workarounds = append(
				report.Workarounds,
				"project_empty_name_accepted_and_adopted",
			)
			return project, nil
		case count > 0:
			return projectWire{}, errors.New(
				"project validation probe created an ambiguous exact-key set",
			)
		}
	} else {
		report.Compatibility.ProjectValidation = outcome
	}

	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		project, decodeErr := decodeLifecycleData[projectWire](result)
		if decodeErr == nil && trackableProject(project) {
			report.Workarounds = append(
				report.Workarounds,
				"project_empty_name_accepted_from_create_response",
			)
			return project, nil
		}
		return projectWire{}, errors.New(
			"project validation probe succeeded without a trackable exact identity",
		)
	}
	if listErr != nil {
		return projectWire{}, errors.New(
			"project validation probe could not verify exact-key absence",
		)
	}
	if !safeValidationRejection(Classify(result.Observation, requestErr)) {
		return projectWire{}, errors.New(
			"project validation probe was ambiguous; refusing a second write",
		)
	}
	return lifecycleCreateProject(ctx, client, report, names)
}

func lifecycleCreateEnvironmentWithValidation(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	projectID string,
	names lifecycleNames,
) (environmentWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		environmentCollectionPath(projectID),
		environmentCollection,
		createEnvironmentRequest{
			Name:        "",
			Key:         names.environmentKey,
			Description: names.description,
		},
	)
	outcome := compatibilityOutcome(result, requestErr, -1)

	project, readErr := lifecycleReadProject(ctx, client, report, projectID)
	if readErr == nil {
		environment, count := environmentByKey(
			project.Environments,
			names.environmentKey,
		)
		outcome.ExactMatchCount = count
		report.Compatibility.EnvironmentValidation = outcome
		switch {
		case count == 1 && trackableEnvironment(environment):
			report.Workarounds = append(
				report.Workarounds,
				"environment_empty_name_accepted_and_adopted",
			)
			return environment, nil
		case count > 0:
			return environmentWire{}, errors.New(
				"environment validation probe created an ambiguous exact-key set",
			)
		}
	} else {
		report.Compatibility.EnvironmentValidation = outcome
	}

	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		environment, decodeErr := decodeLifecycleData[environmentWire](result)
		if decodeErr == nil && trackableEnvironment(environment) {
			report.Workarounds = append(
				report.Workarounds,
				"environment_empty_name_accepted_from_create_response",
			)
			return environment, nil
		}
		return environmentWire{}, errors.New(
			"environment validation probe succeeded without a trackable exact identity",
		)
	}
	if readErr != nil {
		return environmentWire{}, errors.New(
			"environment validation probe could not verify exact-key absence",
		)
	}
	if !safeValidationRejection(Classify(result.Observation, requestErr)) {
		return environmentWire{}, errors.New(
			"environment validation probe was ambiguous; refusing a second write",
		)
	}
	return lifecycleCreateEnvironment(ctx, client, report, projectID, names)
}

func lifecycleProbeDuplicateProject(
	ctx context.Context,
	cfg Config,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	inventory *Inventory,
	inventoryPath string,
	owned projectWire,
	names lifecycleNames,
) error {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		projectCollectionPath,
		projectCollectionPath,
		createProjectRequest{Name: names.projectName, Key: names.projectKey},
	)
	outcome := compatibilityOutcome(result, requestErr, -1)
	distinctResponseIdentity := false
	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		duplicate, decodeErr := decodeLifecycleData[projectWire](result)
		if decodeErr == nil && duplicate.ID != "" && duplicate.ID != owned.ID {
			distinctResponseIdentity = true
			if err := trackUnexpectedProject(
				cfg.Target,
				inventory,
				inventoryPath,
				duplicate.ID,
				names.projectKey,
			); err != nil {
				return err
			}
		}
	}

	projects, listErr := lifecycleListProjects(ctx, client, report)
	if listErr != nil {
		report.Compatibility.ProjectDuplicate = outcome
		if distinctResponseIdentity {
			return errors.New(
				"duplicate project request returned a second identity; cleanup is pending",
			)
		}
		return errors.New(
			"duplicate project request could not be reconciled by exact key",
		)
	}

	exact := projectsByKey(projects, names.projectKey)
	outcome.ExactMatchCount = len(exact)
	report.Compatibility.ProjectDuplicate = outcome
	unexpected := distinctResponseIdentity
	for _, project := range exact {
		if project.ID == owned.ID {
			continue
		}
		unexpected = true
		if err := trackUnexpectedProject(
			cfg.Target,
			inventory,
			inventoryPath,
			project.ID,
			names.projectKey,
		); err != nil {
			return err
		}
	}
	if unexpected || len(exact) != 1 || exact[0].ID != owned.ID {
		return errors.New(
			"duplicate project request changed the exact-key identity set; cleanup is pending",
		)
	}
	if requestErr != nil ||
		Classify(result.Observation, nil) == ClassificationSuccess {
		report.Workarounds = append(
			report.Workarounds,
			"project_duplicate_exact_key_reconciliation",
		)
	}
	return nil
}

func lifecycleProbeDuplicateEnvironment(
	ctx context.Context,
	cfg Config,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	inventory *Inventory,
	inventoryPath string,
	projectID string,
	owned environmentWire,
	names lifecycleNames,
) error {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		environmentCollectionPath(projectID),
		environmentCollection,
		createEnvironmentRequest{
			Name:        names.environmentName,
			Key:         names.environmentKey,
			Description: names.description,
		},
	)
	outcome := compatibilityOutcome(result, requestErr, -1)
	distinctResponseIdentity := false
	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		duplicate, decodeErr := decodeLifecycleData[environmentWire](result)
		if decodeErr == nil && duplicate.ID != "" && duplicate.ID != owned.ID {
			distinctResponseIdentity = true
			if err := trackUnexpectedEnvironment(
				cfg.Target,
				inventory,
				inventoryPath,
				projectID,
				duplicate.ID,
				names.environmentKey,
			); err != nil {
				return err
			}
		}
	}

	project, readErr := lifecycleReadProject(ctx, client, report, projectID)
	if readErr != nil {
		report.Compatibility.EnvironmentDuplicate = outcome
		if distinctResponseIdentity {
			return errors.New(
				"duplicate environment request returned a second identity; cleanup is pending",
			)
		}
		return errors.New(
			"duplicate environment request could not be reconciled by exact key",
		)
	}

	exact := environmentsByKey(project.Environments, names.environmentKey)
	outcome.ExactMatchCount = len(exact)
	report.Compatibility.EnvironmentDuplicate = outcome
	unexpected := distinctResponseIdentity
	for _, environment := range exact {
		if environment.ID == owned.ID {
			continue
		}
		unexpected = true
		if err := trackUnexpectedEnvironment(
			cfg.Target,
			inventory,
			inventoryPath,
			projectID,
			environment.ID,
			names.environmentKey,
		); err != nil {
			return err
		}
	}
	if unexpected || len(exact) != 1 || exact[0].ID != owned.ID {
		return errors.New(
			"duplicate environment request changed the exact-key identity set; cleanup is pending",
		)
	}
	if requestErr != nil ||
		Classify(result.Observation, nil) == ClassificationSuccess {
		report.Workarounds = append(
			report.Workarounds,
			"environment_duplicate_exact_key_reconciliation",
		)
	}
	return nil
}

func compatibilityOutcome(
	result Result,
	requestErr error,
	exactMatchCount int,
) CompatibilityOutcome {
	return CompatibilityOutcome{
		HTTPStatus:      result.Observation.HTTPStatus,
		EnvelopeSuccess: result.Observation.EnvelopeSuccess,
		Classification:  Classify(result.Observation, requestErr),
		ErrorCodes:      append([]string{}, result.Observation.ErrorCodes...),
		ExactMatchCount: exactMatchCount,
	}
}

func safeValidationRejection(classification Classification) bool {
	switch classification {
	case ClassificationValidation,
		ClassificationConflict,
		ClassificationApplicationFailure:
		return true
	default:
		return false
	}
}

func trackUnexpectedProject(
	target Target,
	inventory *Inventory,
	inventoryPath string,
	projectID string,
	projectKey string,
) error {
	if projectID == "" {
		return errors.New(
			"duplicate project response lacked a valid cleanup identity",
		)
	}
	entry := InventoryEntry{
		Target: target,
		Type:   ResourceProject,
		Identity: ResourceIdentity{
			ID:  projectID,
			Key: projectKey,
		},
		TODO: "P0-031",
	}
	return trackUnexpectedEntry(inventory, inventoryPath, entry)
}

func trackUnexpectedEnvironment(
	target Target,
	inventory *Inventory,
	inventoryPath string,
	projectID string,
	environmentID string,
	environmentKey string,
) error {
	if projectID == "" || environmentID == "" {
		return errors.New(
			"duplicate environment response lacked a valid cleanup identity",
		)
	}
	entry := InventoryEntry{
		Target: target,
		Type:   ResourceEnvironment,
		Identity: ResourceIdentity{
			ID:        environmentID,
			Key:       environmentKey,
			ProjectID: projectID,
		},
		TODO: "P0-031",
	}
	return trackUnexpectedEntry(inventory, inventoryPath, entry)
}

func trackUnexpectedEntry(
	inventory *Inventory,
	inventoryPath string,
	entry InventoryEntry,
) error {
	for _, existing := range inventory.Entries {
		if existing.CleanedAt == nil &&
			existing.Target == entry.Target &&
			existing.Type == entry.Type &&
			existing.Identity == entry.Identity {
			return nil
		}
	}
	if err := inventory.Track(entry); err != nil {
		return err
	}
	if err := inventory.Save(inventoryPath); err != nil {
		return err
	}
	return nil
}

func lifecycleCreateProject(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	names lifecycleNames,
) (projectWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		projectCollectionPath,
		projectCollectionPath,
		createProjectRequest{Name: names.projectName, Key: names.projectKey},
	)
	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		project, decodeErr := decodeLifecycleData[projectWire](result)
		if decodeErr == nil && trackableProject(project) {
			return project, nil
		}
	}

	projects, fallbackErr := lifecycleListProjects(ctx, client, report)
	if fallbackErr != nil {
		return projectWire{}, errors.New("project create was ambiguous and exact-key reconciliation failed")
	}
	project, count := projectByKey(projects, names.projectKey)
	if count == 1 && trackableProject(project) {
		report.Workarounds = append(report.Workarounds, "project_create_exact_key_reconciliation")
		return project, nil
	}
	if requestErr != nil {
		return projectWire{}, requestErr
	}
	return projectWire{}, fmt.Errorf(
		"project create classification %s; exact-key reconciliation found %d matches",
		Classify(result.Observation, nil),
		count,
	)
}

func lifecycleCreateEnvironment(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	projectID string,
	names lifecycleNames,
) (environmentWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodPost,
		environmentCollectionPath(projectID),
		environmentCollection,
		createEnvironmentRequest{
			Name:        names.environmentName,
			Key:         names.environmentKey,
			Description: names.description,
		},
	)
	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		environment, decodeErr := decodeLifecycleData[environmentWire](result)
		if decodeErr == nil && trackableEnvironment(environment) {
			return environment, nil
		}
	}

	project, fallbackErr := lifecycleReadProject(ctx, client, report, projectID)
	if fallbackErr != nil {
		return environmentWire{}, errors.New("environment create was ambiguous and exact-key reconciliation failed")
	}
	environment, count := environmentByKey(project.Environments, names.environmentKey)
	if count == 1 && trackableEnvironment(environment) {
		report.Workarounds = append(report.Workarounds, "environment_create_exact_key_reconciliation")
		return environment, nil
	}
	if requestErr != nil {
		return environmentWire{}, requestErr
	}
	return environmentWire{}, fmt.Errorf(
		"environment create classification %s; exact-key reconciliation found %d matches",
		Classify(result.Observation, nil),
		count,
	)
}

func lifecycleListProjects(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
) ([]projectWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		projectCollectionPath,
		projectCollectionPath,
		nil,
	)
	if err := requireLifecycleSuccess(http.MethodGet, projectCollectionPath, result, requestErr); err != nil {
		return nil, err
	}
	return decodeLifecycleData[[]projectWire](result)
}

func lifecycleReadProject(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	projectID string,
) (projectWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		projectPath(projectID),
		projectItemTemplate,
		nil,
	)
	if err := requireLifecycleSuccess(http.MethodGet, projectItemTemplate, result, requestErr); err != nil {
		return projectWire{}, err
	}
	return decodeLifecycleData[projectWire](result)
}

func lifecycleReadEnvironment(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	projectID string,
	environmentID string,
) (environmentWire, error) {
	result, requestErr := lifecycleRequest(
		ctx,
		client,
		report,
		http.MethodGet,
		environmentPath(projectID, environmentID),
		environmentItem,
		nil,
	)
	if err := requireLifecycleSuccess(http.MethodGet, environmentItem, result, requestErr); err != nil {
		return environmentWire{}, err
	}
	environment, err := decodeLifecycleData[environmentWire](result)
	if err != nil {
		return environmentWire{}, err
	}
	if environment.ID == "" {
		environment.ID = environmentID
	}
	if environment.ProjectID == "" {
		environment.ProjectID = projectID
	}
	return environment, nil
}

func lifecycleRequest(
	ctx context.Context,
	client *Client,
	report *ProjectEnvironmentLifecycleReport,
	method string,
	requestPath string,
	pathTemplate string,
	body interface{},
) (Result, error) {
	result, err := client.DoJSONAt(ctx, method, requestPath, pathTemplate, body)
	if result.Observation.HTTPStatus != 0 {
		report.Observations = append(report.Observations, result.Observation)
	}
	return result, err
}

func requireLifecycleSuccess(
	method string,
	pathTemplate string,
	result Result,
	requestErr error,
) error {
	if requestErr != nil {
		return requestErr
	}
	classification := Classify(result.Observation, nil)
	if classification != ClassificationSuccess {
		return fmt.Errorf("%s %s returned classification %s", method, pathTemplate, classification)
	}
	return nil
}

func requireDeleteOrVerifiedAbsence(
	method string,
	pathTemplate string,
	result Result,
	requestErr error,
	report *ProjectEnvironmentLifecycleReport,
	workaround string,
) error {
	if requestErr == nil && Classify(result.Observation, nil) == ClassificationSuccess {
		return nil
	}
	report.Workarounds = append(report.Workarounds, workaround)
	return nil
}

func decodeLifecycleData[T any](result Result) (T, error) {
	var value T
	if len(result.Envelope.Data) == 0 ||
		bytes.Equal(bytes.TrimSpace(result.Envelope.Data), []byte("null")) {
		return value, errors.New("successful lifecycle response omitted data")
	}
	if err := json.Unmarshal(result.Envelope.Data, &value); err != nil {
		return value, errors.New("decode lifecycle response data")
	}
	return value, nil
}

func namesForPrefix(prefix string) lifecycleNames {
	return lifecycleNames{
		projectName:        prefix + " project",
		projectUpdatedName: prefix + " project updated",
		projectKey:         prefix,
		environmentName:    prefix + " environment",
		environmentUpdated: prefix + " environment updated",
		environmentKey:     prefix,
		description:        "Terraform provider Phase 0 probe",
		updatedDescription: "Terraform provider Phase 0 probe updated",
	}
}

func trackableProject(project projectWire) bool {
	return project.ID != ""
}

func trackableEnvironment(environment environmentWire) bool {
	return environment.ID != ""
}

func projectPath(projectID string) string {
	return projectCollectionPath + "/" + url.PathEscape(projectID)
}

func environmentCollectionPath(projectID string) string {
	return projectPath(projectID) + "/envs"
}

func environmentPath(projectID string, environmentID string) string {
	return environmentCollectionPath(projectID) + "/" + url.PathEscape(environmentID)
}

func countProjectsByKey(projects []projectWire, key string) int {
	_, count := projectByKey(projects, key)
	return count
}

func projectByKey(projects []projectWire, key string) (projectWire, int) {
	var match projectWire
	count := 0
	for _, project := range projects {
		if project.Key == key {
			match = project
			count++
		}
	}
	return match, count
}

func projectsByKey(projects []projectWire, key string) []projectWire {
	matches := make([]projectWire, 0)
	for _, project := range projects {
		if project.Key == key {
			matches = append(matches, project)
		}
	}
	return matches
}

func countProjectsByID(projects []projectWire, id string) int {
	count := 0
	for _, project := range projects {
		if project.ID == id {
			count++
		}
	}
	return count
}

func countEnvironmentsByKey(environments []environmentWire, key string) int {
	_, count := environmentByKey(environments, key)
	return count
}

func environmentByKey(environments []environmentWire, key string) (environmentWire, int) {
	var match environmentWire
	count := 0
	for _, environment := range environments {
		if environment.Key == key {
			match = environment
			count++
		}
	}
	return match, count
}

func environmentsByKey(environments []environmentWire, key string) []environmentWire {
	matches := make([]environmentWire, 0)
	for _, environment := range environments {
		if environment.Key == key {
			matches = append(matches, environment)
		}
	}
	return matches
}

func environmentByID(environments []environmentWire, id string) (environmentWire, int) {
	var match environmentWire
	count := 0
	for _, environment := range environments {
		if environment.ID == id {
			match = environment
			count++
		}
	}
	return match, count
}

func canonicalizeProject(project projectWire) canonicalProject {
	canonical := canonicalProject{
		ID:           project.ID,
		Name:         project.Name,
		Key:          project.Key,
		Environments: make([]canonicalEnvironment, 0, len(project.Environments)),
	}
	for _, environment := range project.Environments {
		canonical.Environments = append(
			canonical.Environments,
			canonicalizeEnvironment(environment),
		)
	}
	return canonical
}

func canonicalizeEnvironment(environment environmentWire) canonicalEnvironment {
	return canonicalEnvironment{
		ID:          environment.ID,
		ProjectID:   environment.ProjectID,
		Name:        environment.Name,
		Key:         environment.Key,
		Description: environment.Description,
		Settings:    environment.Settings,
		Secrets:     summarizeSecrets(environment.Secrets),
	}
}

func summarizeEnvironmentShapes(environments []environmentWire) []EnvironmentShapeSummary {
	summaries := make([]EnvironmentShapeSummary, 0, len(environments))
	for index, environment := range environments {
		summary := EnvironmentShapeSummary{
			Position:           index,
			Name:               environment.Name,
			Key:                environment.Key,
			DescriptionPresent: environment.Description != "",
			SettingsPresent:    environment.Settings != nil,
			SecretMetadata:     summarizeSecrets(environment.Secrets),
		}
		if environment.Settings != nil {
			summary.RequireChangeComment = environment.Settings.RequireChangeComment
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func summarizeSecrets(secrets []secretMetadataWire) SecretMetadataSummary {
	summary := SecretMetadataSummary{Count: len(secrets)}
	typeClasses := map[string]struct{}{}
	valueTypes := map[string]struct{}{}
	for _, secret := range secrets {
		if secret.IDPresent {
			summary.IDFieldPresent++
		}
		if secret.NamePresent {
			summary.NameFieldPresent++
		}
		if secret.TypePresent {
			summary.TypeFieldPresent++
			if secret.TypeClass != "" {
				typeClasses[secret.TypeClass] = struct{}{}
			}
		}
		if secret.ValuePresent {
			summary.ValueFieldPresent++
			if secret.ValueJSONType != "" {
				valueTypes[secret.ValueJSONType] = struct{}{}
			}
		}
	}
	for value := range typeClasses {
		summary.TypeClasses = append(summary.TypeClasses, value)
	}
	for value := range valueTypes {
		summary.ValueJSONTypes = append(summary.ValueJSONTypes, value)
	}
	sort.Strings(summary.TypeClasses)
	sort.Strings(summary.ValueJSONTypes)
	return summary
}

func jsonValueType(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "missing"
	}
	switch trimmed[0] {
	case '"':
		return "string"
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}
