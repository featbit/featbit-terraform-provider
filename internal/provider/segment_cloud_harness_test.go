// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/google/uuid"
)

const (
	cloudSegmentReferenceProperty = "User is in segment"
	cloudSegmentReferenceOperator = "IsOneOf"
)

type cloudSegmentRecord struct {
	id            string
	environmentID string
	key           string
}

// cloudSegmentInventory owns only uniquely keyed acceptance objects. It
// deletes Feature Flags before Segments and Segments before their test-owned
// Environment/Project parents. The exact-key discovery pass also covers a
// successful Create whose response never reached Terraform state.
type cloudSegmentInventory struct {
	api     *client.Client
	parents *cloudAcceptanceInventory
	flags   *cloudFeatureFlagInventory

	mu          sync.Mutex
	segmentKeys map[string]struct{}
	segments    map[string]cloudSegmentRecord
}

func newCloudSegmentInventory(
	apiClient *client.Client,
	projectKeys []string,
	environmentKeys []string,
	featureFlagKeys []string,
	segmentKeys []string,
) *cloudSegmentInventory {
	parents := newCloudAcceptanceInventory(apiClient, projectKeys, environmentKeys)
	inventory := &cloudSegmentInventory{
		api:         apiClient,
		parents:     parents,
		flags:       newCloudFeatureFlagInventoryWithParents(apiClient, parents, featureFlagKeys),
		segmentKeys: make(map[string]struct{}, len(segmentKeys)),
		segments:    make(map[string]cloudSegmentRecord),
	}
	for _, key := range segmentKeys {
		inventory.segmentKeys[key] = struct{}{}
	}
	return inventory
}

func (i *cloudSegmentInventory) registerSegment(
	environmentID string,
	segmentID string,
	key string,
) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.segments[cloudSegmentInventoryIdentity(environmentID, key)] = cloudSegmentRecord{
		id:            segmentID,
		environmentID: environmentID,
		key:           key,
	}
}

func (i *cloudSegmentInventory) forgetSegment(environmentID string, key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.segments, cloudSegmentInventoryIdentity(environmentID, key))
}

func (i *cloudSegmentInventory) deleteSegment(
	ctx context.Context,
	environmentID string,
	segmentID string,
	key string,
) error {
	match, status, err := i.api.ResolveSegment(ctx, environmentID, client.SegmentIdentity{
		ID:  segmentID,
		Key: key,
	})
	if err != nil {
		return errors.New("Cloud Segment starting status could not be resolved")
	}
	if status == client.SegmentStatusAbsent {
		i.forgetSegment(environmentID, key)
		return nil
	}
	if match.Type != client.SegmentTypeEnvironmentSpecific ||
		!client.EqualUUID(match.EnvironmentID, environmentID) {
		return errors.New("Cloud Segment cleanup refused a non-owned taxonomy")
	}
	if err := cloudWaitSegmentReferencesEmpty(ctx, i.api, environmentID, match.ID); err != nil {
		return err
	}

	if status == client.SegmentStatusActive {
		archiveErr := i.api.ArchiveSegment(ctx, environmentID, match.ID)
		_, status, err = cloudWaitSegmentStatus(
			ctx,
			i.api,
			environmentID,
			client.SegmentIdentity{ID: match.ID, Key: key},
			client.SegmentStatusArchived,
		)
		if err != nil || status != client.SegmentStatusArchived {
			if archiveErr != nil {
				return errors.New("Cloud Segment archive and reconciliation failed")
			}
			return errors.New("Cloud Segment archive was not confirmed")
		}
	}
	if status != client.SegmentStatusArchived {
		return errors.New("Cloud Segment cleanup status was not authoritative")
	}

	deleteErr := i.api.DeleteSegment(ctx, environmentID, match.ID)
	_, status, err = cloudWaitSegmentStatus(
		ctx,
		i.api,
		environmentID,
		client.SegmentIdentity{ID: match.ID, Key: key},
		client.SegmentStatusAbsent,
	)
	if err != nil || status != client.SegmentStatusAbsent {
		if deleteErr != nil {
			return errors.New("Cloud Segment permanent delete and absence proof failed")
		}
		return errors.New("Cloud Segment exact absence proof failed")
	}
	i.forgetSegment(environmentID, key)
	return nil
}

func (i *cloudSegmentInventory) verifySegmentsAbsent(
	ctx context.Context,
	environmentID string,
	keys []string,
) error {
	for _, key := range keys {
		_, status, err := i.api.ResolveSegment(
			ctx,
			environmentID,
			client.SegmentIdentity{Key: key},
		)
		if err != nil || status != client.SegmentStatusAbsent {
			return errors.New("Cloud Segment final active/archived views were not exact zero")
		}
		i.forgetSegment(environmentID, key)
	}
	return nil
}

func (i *cloudSegmentInventory) cleanupAndVerify(ctx context.Context) error {
	projects, err := i.api.ListProjects(ctx)
	if err != nil {
		return errors.New("Cloud Segment cleanup could not discover test-owned parents")
	}

	existingEnvironments := make(map[string]struct{})
	for _, project := range projects {
		if _, tracked := i.parents.projectKeys[project.Key]; !tracked {
			continue
		}
		i.parents.registerProject(project.ID, project.Key)
		for _, environment := range project.Environments {
			if _, tracked := i.parents.environmentKeys[environment.Key]; !tracked {
				continue
			}
			i.parents.registerEnvironment(project.ID, environment.ID, environment.Key)
			existingEnvironments[environment.ID] = struct{}{}
		}
	}

	// Discover only the unique keys registered before the test, and only in
	// exact test-owned parents. No unrelated Feature Flag or Segment is read.
	for environmentID := range existingEnvironments {
		for key := range i.flags.flagKeys {
			flag, status, resolveErr := i.api.ResolveFeatureFlag(ctx, environmentID, key)
			if resolveErr != nil {
				return errors.New("Cloud Segment cleanup could not resolve a test-owned Feature Flag key")
			}
			if status == client.FeatureFlagStatusActive ||
				status == client.FeatureFlagStatusArchived {
				i.flags.registerFeatureFlag(environmentID, flag.ID, key)
			}
		}
		for key := range i.segmentKeys {
			segment, status, resolveErr := i.api.ResolveSegment(
				ctx,
				environmentID,
				client.SegmentIdentity{Key: key},
			)
			if resolveErr != nil {
				return errors.New("Cloud Segment cleanup could not resolve a test-owned Segment key")
			}
			if status == client.SegmentStatusActive || status == client.SegmentStatusArchived {
				i.registerSegment(environmentID, segment.ID, key)
			}
		}
	}

	i.flags.mu.Lock()
	for identity, record := range i.flags.flags {
		if _, exists := existingEnvironments[record.environmentID]; !exists {
			delete(i.flags.flags, identity)
		}
	}
	flags := make([]cloudFeatureFlagRecord, 0, len(i.flags.flags))
	for _, record := range i.flags.flags {
		flags = append(flags, record)
	}
	i.flags.mu.Unlock()
	sort.Slice(flags, func(left, right int) bool {
		return featureFlagInventoryIdentity(flags[left].environmentID, flags[left].key) <
			featureFlagInventoryIdentity(flags[right].environmentID, flags[right].key)
	})

	i.mu.Lock()
	for identity, record := range i.segments {
		if _, exists := existingEnvironments[record.environmentID]; !exists {
			delete(i.segments, identity)
		}
	}
	segments := make([]cloudSegmentRecord, 0, len(i.segments))
	for _, record := range i.segments {
		segments = append(segments, record)
	}
	i.mu.Unlock()
	sort.Slice(segments, func(left, right int) bool {
		return cloudSegmentInventoryIdentity(segments[left].environmentID, segments[left].key) <
			cloudSegmentInventoryIdentity(segments[right].environmentID, segments[right].key)
	})

	cleanupFailures := 0
	for _, record := range flags {
		if err := i.flags.deleteFeatureFlag(ctx, record.environmentID, record.key); err != nil {
			cleanupFailures++
		}
	}
	for environmentID := range existingEnvironments {
		keys := sortedCloudInventoryKeys(i.flags.flagKeys)
		if err := i.flags.verifyFeatureFlagsAbsent(ctx, environmentID, keys); err != nil {
			cleanupFailures++
		}
	}
	for _, record := range segments {
		if err := i.deleteSegment(ctx, record.environmentID, record.id, record.key); err != nil {
			cleanupFailures++
		}
	}
	for environmentID := range existingEnvironments {
		keys := sortedCloudInventoryKeys(i.segmentKeys)
		if err := i.verifySegmentsAbsent(ctx, environmentID, keys); err != nil {
			cleanupFailures++
		}
	}
	if err := i.parents.cleanupAndVerify(ctx); err != nil {
		cleanupFailures++
	}

	i.flags.mu.Lock()
	pendingFlags := len(i.flags.flags)
	i.flags.mu.Unlock()
	i.mu.Lock()
	pendingSegments := len(i.segments)
	i.mu.Unlock()
	if cleanupFailures != 0 || pendingFlags != 0 || pendingSegments != 0 {
		return errors.New("Cloud Segment cleanup retained an exact object or pending owner")
	}
	return nil
}

func cloudSegmentInventoryIdentity(environmentID string, key string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	return canonicalEnvironmentID + "\x00" + key
}

func sortedCloudInventoryKeys(keys map[string]struct{}) []string {
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func cloudWaitSegmentReferencesEmpty(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	segmentID string,
) error {
	for attempt := 0; attempt < 30; attempt++ {
		references, err := apiClient.GetSegmentFlagReferences(ctx, environmentID, segmentID)
		if err == nil && len(references) == 0 {
			return nil
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return errors.New("Cloud Segment cleanup retained an exact Feature Flag reference")
}

func cloudWaitSegmentStatus(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	identity client.SegmentIdentity,
	want client.SegmentStatus,
) (client.SegmentMatch, client.SegmentStatus, error) {
	var last client.SegmentStatus
	for attempt := 0; attempt < 30; attempt++ {
		match, status, err := apiClient.ResolveSegment(ctx, environmentID, identity)
		last = status
		if err == nil && status == want {
			return match, status, nil
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return client.SegmentMatch{}, last, errors.New("Cloud Segment status did not converge")
}

func cloudObservationDelay(ctx context.Context) bool {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type cloudFeatureFlagTargetingClient struct {
	base       url.URL
	token      string
	httpClient http.Client
}

func newCloudFeatureFlagTargetingClient(
	apiURL *url.URL,
	accessToken string,
) *cloudFeatureFlagTargetingClient {
	return &cloudFeatureFlagTargetingClient{
		base:  *apiURL,
		token: accessToken,
		httpClient: http.Client{
			Timeout: client.DefaultHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type cloudFeatureFlagTargeting struct {
	DisabledVariationID   string          `json:"disabledVariationId"`
	TargetUsers           json.RawMessage `json:"targetUsers"`
	Rules                 json.RawMessage `json:"rules"`
	Fallthrough           json.RawMessage `json:"fallthrough"`
	ExptIncludeAllTargets bool            `json:"exptIncludeAllTargets"`
}

type cloudFeatureFlagExactTargeting struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"envId"`
	Key           string `json:"key"`
	Revision      string `json:"revision"`
	Variations    []struct {
		ID string `json:"id"`
	} `json:"variations"`
	DisabledVariationID   string          `json:"disabledVariationId"`
	TargetUsers           json.RawMessage `json:"targetUsers"`
	Rules                 json.RawMessage `json:"rules"`
	Fallthrough           json.RawMessage `json:"fallthrough"`
	ExptIncludeAllTargets bool            `json:"exptIncludeAllTargets"`
}

func (cloudFeatureFlagExactTargeting) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "provider.cloudFeatureFlagExactTargeting{redacted}")
}

func (f cloudFeatureFlagExactTargeting) targeting() cloudFeatureFlagTargeting {
	return cloudFeatureFlagTargeting{
		DisabledVariationID:   f.DisabledVariationID,
		TargetUsers:           append(json.RawMessage(nil), f.TargetUsers...),
		Rules:                 append(json.RawMessage(nil), f.Rules...),
		Fallthrough:           append(json.RawMessage(nil), f.Fallthrough...),
		ExptIncludeAllTargets: f.ExptIncludeAllTargets,
	}
}

type cloudFeatureFlagReferenceSnapshot struct {
	environmentID string
	flagID        string
	flagKey       string
	segmentID     string
	ruleID        string
	baseline      cloudFeatureFlagTargeting
	fingerprint   cloudSettingsFingerprint
}

type cloudFeatureFlagReferenceRule struct {
	ID             string                          `json:"id"`
	Name           string                          `json:"name"`
	DispatchKey    string                          `json:"dispatchKey"`
	IncludedInExpt bool                            `json:"includedInExpt"`
	Conditions     []cloudFeatureFlagReferenceCond `json:"conditions"`
	Variations     []cloudFeatureFlagReferenceRoll `json:"variations"`
}

type cloudFeatureFlagReferenceCond struct {
	ID       string `json:"id"`
	Property string `json:"property"`
	Operator string `json:"op"`
	Value    string `json:"value"`
}

type cloudFeatureFlagReferenceRoll struct {
	ID          string    `json:"id"`
	Rollout     []float64 `json:"rollout"`
	ExptRollout float64   `json:"exptRollout"`
}

func (c *cloudFeatureFlagTargetingClient) addSegmentReference(
	ctx context.Context,
	environmentID string,
	flagKey string,
	segmentID string,
) (cloudFeatureFlagReferenceSnapshot, error) {
	flag, err := c.get(ctx, environmentID, flagKey)
	if err != nil || len(flag.Variations) == 0 || flag.Variations[0].ID == "" {
		return cloudFeatureFlagReferenceSnapshot{}, errors.New("Cloud Feature Flag reference baseline was unavailable")
	}
	baseline := flag.targeting()
	fingerprint, err := fingerprintCloudFeatureFlagTargeting(baseline)
	if err != nil {
		return cloudFeatureFlagReferenceSnapshot{}, err
	}

	ruleID := uuid.NewString()
	conditionID := uuid.NewString()
	encodedIDs, _ := json.Marshal([]string{segmentID})
	rule, err := json.Marshal(cloudFeatureFlagReferenceRule{
		ID:             ruleID,
		Name:           "Terraform Cloud Segment reference",
		DispatchKey:    "keyId",
		IncludedInExpt: false,
		Conditions: []cloudFeatureFlagReferenceCond{{
			ID:       conditionID,
			Property: cloudSegmentReferenceProperty,
			Operator: cloudSegmentReferenceOperator,
			Value:    string(encodedIDs),
		}},
		Variations: []cloudFeatureFlagReferenceRoll{{
			ID:          flag.Variations[0].ID,
			Rollout:     []float64{0, 1},
			ExptRollout: 0,
		}},
	})
	if err != nil {
		return cloudFeatureFlagReferenceSnapshot{}, errors.New("Cloud Feature Flag reference could not be encoded")
	}
	rules, err := cloudFeatureFlagRawRules(baseline.Rules)
	if err != nil {
		return cloudFeatureFlagReferenceSnapshot{}, err
	}
	rules = append(rules, json.RawMessage(rule))
	withReference := baseline
	withReference.Rules, err = json.Marshal(rules)
	if err != nil {
		return cloudFeatureFlagReferenceSnapshot{}, errors.New("Cloud Feature Flag targeting could not be encoded")
	}

	snapshot := cloudFeatureFlagReferenceSnapshot{
		environmentID: environmentID,
		flagID:        flag.ID,
		flagKey:       flagKey,
		segmentID:     segmentID,
		ruleID:        ruleID,
		baseline:      baseline,
		fingerprint:   fingerprint,
	}
	_ = c.put(ctx, environmentID, flagKey, flag.Revision, withReference)
	for attempt := 0; attempt < 30; attempt++ {
		observed, readErr := c.get(ctx, environmentID, flagKey)
		if readErr == nil && cloudFeatureFlagHasExactReference(observed, snapshot) {
			return snapshot, nil
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return cloudFeatureFlagReferenceSnapshot{}, errors.New("Cloud Feature Flag reference was not confirmed")
}

func (c *cloudFeatureFlagTargetingClient) removeSegmentReference(
	ctx context.Context,
	snapshot cloudFeatureFlagReferenceSnapshot,
) error {
	current, err := c.get(ctx, snapshot.environmentID, snapshot.flagKey)
	if err != nil || !cloudFeatureFlagTargetingOnlyAddsReference(current, snapshot) {
		return errors.New("Cloud Feature Flag reference removal refused unexpected targeting drift")
	}
	_ = c.put(
		ctx,
		snapshot.environmentID,
		snapshot.flagKey,
		current.Revision,
		snapshot.baseline,
	)
	for attempt := 0; attempt < 30; attempt++ {
		if c.verifyBaseline(ctx, snapshot) == nil {
			return nil
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return errors.New("Cloud Feature Flag reference removal was not confirmed")
}

func (c *cloudFeatureFlagTargetingClient) verifyBaseline(
	ctx context.Context,
	snapshot cloudFeatureFlagReferenceSnapshot,
) error {
	flag, err := c.get(ctx, snapshot.environmentID, snapshot.flagKey)
	if err != nil || !client.EqualUUID(flag.ID, snapshot.flagID) {
		return errors.New("Cloud Feature Flag baseline identity was not retained")
	}
	fingerprint, err := fingerprintCloudFeatureFlagTargeting(flag.targeting())
	if err != nil || cloudSettingsFingerprintDifference(snapshot.fingerprint, fingerprint) != "" {
		return errors.New("Cloud Feature Flag baseline targeting was not retained")
	}
	return nil
}

func (c *cloudFeatureFlagTargetingClient) get(
	ctx context.Context,
	environmentID string,
	flagKey string,
) (cloudFeatureFlagExactTargeting, error) {
	data, err := c.do(ctx, http.MethodGet, environmentID, flagKey, nil)
	if err != nil {
		return cloudFeatureFlagExactTargeting{}, err
	}
	var flag cloudFeatureFlagExactTargeting
	if json.Unmarshal(data, &flag) != nil || !client.ValidUUID(flag.ID) ||
		!client.EqualUUID(flag.EnvironmentID, environmentID) || flag.Key != flagKey ||
		!client.ValidUUID(flag.Revision) || flag.DisabledVariationID == "" ||
		!json.Valid(flag.TargetUsers) || !json.Valid(flag.Rules) ||
		!json.Valid(flag.Fallthrough) {
		return cloudFeatureFlagExactTargeting{}, errors.New("Cloud Feature Flag targeting response was invalid")
	}
	return flag, nil
}

func (c *cloudFeatureFlagTargetingClient) put(
	ctx context.Context,
	environmentID string,
	flagKey string,
	revision string,
	targeting cloudFeatureFlagTargeting,
) error {
	if !client.ValidUUID(revision) {
		return errors.New("Cloud Feature Flag targeting revision was invalid")
	}
	payload := struct {
		Revision  string                    `json:"revision"`
		Targeting cloudFeatureFlagTargeting `json:"targeting"`
	}{
		Revision:  revision,
		Targeting: targeting,
	}
	_, err := c.do(ctx, http.MethodPut, environmentID, flagKey, payload)
	return err
}

func (c *cloudFeatureFlagTargetingClient) do(
	ctx context.Context,
	method string,
	environmentID string,
	flagKey string,
	payload any,
) (json.RawMessage, error) {
	if !client.ValidUUID(environmentID) || flagKey == "" || c.token == "" {
		return nil, errors.New("Cloud Feature Flag targeting request identity was invalid")
	}
	endpoint, err := url.JoinPath(
		c.base.String(),
		"envs",
		environmentID,
		"feature-flags",
		flagKey,
	)
	if err != nil {
		return nil, errors.New("Cloud Feature Flag targeting endpoint was invalid")
	}
	if method == http.MethodPut {
		endpoint, err = url.JoinPath(endpoint, "targeting")
		if err != nil {
			return nil, errors.New("Cloud Feature Flag targeting endpoint was invalid")
		}
	}

	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, errors.New("Cloud Feature Flag targeting request could not be encoded")
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, errors.New("Cloud Feature Flag targeting request could not be constructed")
	}
	request.Header.Set("Authorization", c.token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, errors.New("Cloud Feature Flag targeting request failed")
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, client.MaxResponseBytes+1))
	if err != nil || int64(len(content)) > client.MaxResponseBytes {
		return nil, errors.New("Cloud Feature Flag targeting response exceeded its safe boundary")
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices ||
		json.Unmarshal(content, &envelope) != nil || !envelope.Success {
		return nil, errors.New("Cloud Feature Flag targeting response was unsuccessful")
	}
	return envelope.Data, nil
}

func cloudFeatureFlagRawRules(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var rules []json.RawMessage
	if json.Unmarshal(raw, &rules) != nil {
		return nil, errors.New("Cloud Feature Flag rules were invalid")
	}
	return rules, nil
}

func cloudFeatureFlagHasExactReference(
	flag cloudFeatureFlagExactTargeting,
	snapshot cloudFeatureFlagReferenceSnapshot,
) bool {
	if !client.EqualUUID(flag.ID, snapshot.flagID) || flag.Key != snapshot.flagKey {
		return false
	}
	rules, err := cloudFeatureFlagRawRules(flag.Rules)
	if err != nil {
		return false
	}
	for _, raw := range rules {
		var rule cloudFeatureFlagReferenceRule
		if json.Unmarshal(raw, &rule) != nil || rule.ID != snapshot.ruleID ||
			len(rule.Conditions) != 1 {
			continue
		}
		condition := rule.Conditions[0]
		if condition.Property != cloudSegmentReferenceProperty ||
			condition.Operator != cloudSegmentReferenceOperator {
			return false
		}
		var ids []string
		return json.Unmarshal([]byte(condition.Value), &ids) == nil &&
			len(ids) == 1 && client.EqualUUID(ids[0], snapshot.segmentID)
	}
	return false
}

func cloudFeatureFlagTargetingOnlyAddsReference(
	flag cloudFeatureFlagExactTargeting,
	snapshot cloudFeatureFlagReferenceSnapshot,
) bool {
	if !cloudFeatureFlagHasExactReference(flag, snapshot) {
		return false
	}
	current := flag.targeting()
	currentRules, err := cloudFeatureFlagRawRules(current.Rules)
	if err != nil {
		return false
	}
	baselineRules, err := cloudFeatureFlagRawRules(snapshot.baseline.Rules)
	if err != nil {
		return false
	}
	filtered := make([]json.RawMessage, 0, len(currentRules))
	for _, raw := range currentRules {
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &identity) == nil && identity.ID == snapshot.ruleID {
			continue
		}
		filtered = append(filtered, raw)
	}
	if len(filtered) != len(baselineRules) {
		return false
	}
	for index := range filtered {
		if !equalCloudJSON(filtered[index], baselineRules[index]) {
			return false
		}
	}
	current.Rules = snapshot.baseline.Rules
	fingerprint, err := fingerprintCloudFeatureFlagTargeting(current)
	return err == nil &&
		cloudSettingsFingerprintDifference(snapshot.fingerprint, fingerprint) == ""
}

func equalCloudJSON(left json.RawMessage, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil &&
		json.Unmarshal(right, &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

func fingerprintCloudFeatureFlagTargeting(
	targeting cloudFeatureFlagTargeting,
) (cloudSettingsFingerprint, error) {
	encoded, err := json.Marshal(targeting)
	if err != nil {
		return nil, errors.New("Cloud Feature Flag targeting could not be fingerprinted")
	}
	var normalized any
	if json.Unmarshal(encoded, &normalized) != nil {
		return nil, errors.New("Cloud Feature Flag targeting could not be fingerprinted")
	}
	fingerprint := make(cloudSettingsFingerprint)
	if err := addCloudSettingsFingerprint(fingerprint, "$", normalized); err != nil {
		return nil, errors.New("Cloud Feature Flag targeting could not be fingerprinted")
	}
	return fingerprint, nil
}
