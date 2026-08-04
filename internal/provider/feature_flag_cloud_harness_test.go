// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"

	"github.com/featbit/terraform-provider-featbit/internal/client"
)

type cloudFeatureFlagRecord struct {
	id            string
	environmentID string
	key           string
}

type cloudFeatureFlagInventory struct {
	api     *client.Client
	parents *cloudAcceptanceInventory

	mu       sync.Mutex
	flagKeys map[string]struct{}
	flags    map[string]cloudFeatureFlagRecord
}

func newCloudFeatureFlagInventory(
	apiClient *client.Client,
	projectKeys []string,
	environmentKeys []string,
	featureFlagKeys []string,
) *cloudFeatureFlagInventory {
	return newCloudFeatureFlagInventoryWithParents(
		apiClient,
		newCloudAcceptanceInventory(apiClient, projectKeys, environmentKeys),
		featureFlagKeys,
	)
}

func newCloudFeatureFlagInventoryWithParents(
	apiClient *client.Client,
	parents *cloudAcceptanceInventory,
	featureFlagKeys []string,
) *cloudFeatureFlagInventory {
	inventory := &cloudFeatureFlagInventory{
		api:      apiClient,
		parents:  parents,
		flagKeys: make(map[string]struct{}, len(featureFlagKeys)),
		flags:    make(map[string]cloudFeatureFlagRecord),
	}
	for _, key := range featureFlagKeys {
		inventory.flagKeys[key] = struct{}{}
	}
	return inventory
}

func (i *cloudFeatureFlagInventory) registerProject(projectID string, key string) {
	i.parents.registerProject(projectID, key)
}

func (i *cloudFeatureFlagInventory) registerEnvironment(
	projectID string,
	environmentID string,
	key string,
) {
	i.parents.registerEnvironment(projectID, environmentID, key)
}

func (i *cloudFeatureFlagInventory) registerFeatureFlag(
	environmentID string,
	featureFlagID string,
	key string,
) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.flags[featureFlagInventoryIdentity(environmentID, key)] = cloudFeatureFlagRecord{
		id:            featureFlagID,
		environmentID: environmentID,
		key:           key,
	}
}

func (i *cloudFeatureFlagInventory) forgetFeatureFlag(environmentID string, key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.flags, featureFlagInventoryIdentity(environmentID, key))
}

func (i *cloudFeatureFlagInventory) deleteFeatureFlag(
	ctx context.Context,
	environmentID string,
	key string,
) error {
	_, status, err := i.api.ResolveFeatureFlag(ctx, environmentID, key)
	if err != nil {
		return fmt.Errorf("Cloud Feature Flag starting status could not be resolved")
	}
	if status == client.FeatureFlagStatusAbsent {
		i.forgetFeatureFlag(environmentID, key)
		return nil
	}
	if status == client.FeatureFlagStatusActive {
		archiveErr := i.api.ArchiveFeatureFlag(ctx, environmentID, key)
		_, status, err = i.api.ResolveFeatureFlag(ctx, environmentID, key)
		if err != nil || status != client.FeatureFlagStatusArchived {
			if archiveErr != nil {
				return fmt.Errorf("Cloud Feature Flag archive and reconciliation failed")
			}
			return fmt.Errorf("Cloud Feature Flag archive was not confirmed")
		}
	}
	if status != client.FeatureFlagStatusArchived {
		return fmt.Errorf("Cloud Feature Flag cleanup status was not authoritative")
	}

	deleteErr := i.api.DeleteFeatureFlag(ctx, environmentID, key)
	_, status, err = i.api.ResolveFeatureFlag(ctx, environmentID, key)
	if err != nil || status != client.FeatureFlagStatusAbsent {
		if deleteErr != nil {
			return fmt.Errorf("Cloud Feature Flag permanent delete and absence proof failed")
		}
		return fmt.Errorf("Cloud Feature Flag exact absence proof failed")
	}
	i.forgetFeatureFlag(environmentID, key)
	return nil
}

func (i *cloudFeatureFlagInventory) verifyFeatureFlagsAbsent(
	ctx context.Context,
	environmentID string,
	keys []string,
) error {
	for _, key := range keys {
		_, status, err := i.api.ResolveFeatureFlag(ctx, environmentID, key)
		if err != nil || status != client.FeatureFlagStatusAbsent {
			return fmt.Errorf("Cloud Feature Flag final active/archived views were not exact zero")
		}
		i.forgetFeatureFlag(environmentID, key)
	}
	return nil
}

func (i *cloudFeatureFlagInventory) cleanupAndVerify(ctx context.Context) error {
	projects, err := i.api.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("Cloud Feature Flag cleanup could not discover test-owned parents")
	}

	existingEnvironments := make(map[string]struct{})
	for _, project := range projects {
		if _, tracked := i.parents.projectKeys[project.Key]; !tracked {
			continue
		}
		i.registerProject(project.ID, project.Key)
		for _, environment := range project.Environments {
			if _, tracked := i.parents.environmentKeys[environment.Key]; !tracked {
				continue
			}
			i.registerEnvironment(project.ID, environment.ID, environment.Key)
			existingEnvironments[environment.ID] = struct{}{}
		}
	}

	// A successful Create may not return state. Resolve only the unique keys
	// registered before the test, and only inside exact test-owned parents.
	for environmentID := range existingEnvironments {
		for key := range i.flagKeys {
			flag, status, resolveErr := i.api.ResolveFeatureFlag(ctx, environmentID, key)
			if resolveErr != nil {
				return fmt.Errorf("Cloud Feature Flag cleanup could not resolve a test-owned key")
			}
			if status == client.FeatureFlagStatusActive ||
				status == client.FeatureFlagStatusArchived {
				i.registerFeatureFlag(environmentID, flag.ID, key)
			}
		}
	}

	i.mu.Lock()
	for identity, record := range i.flags {
		if _, exists := existingEnvironments[record.environmentID]; !exists {
			// Exact parent collection absence is authoritative for a stale
			// in-memory child record left after parent replacement or cascade.
			delete(i.flags, identity)
		}
	}
	flags := make([]cloudFeatureFlagRecord, 0, len(i.flags))
	for _, record := range i.flags {
		flags = append(flags, record)
	}
	i.mu.Unlock()

	cleanupFailures := 0
	for _, record := range flags {
		if err := i.deleteFeatureFlag(ctx, record.environmentID, record.key); err != nil {
			cleanupFailures++
		}
	}
	if err := i.parents.cleanupAndVerify(ctx); err != nil {
		cleanupFailures++
	}

	i.mu.Lock()
	pending := len(i.flags)
	i.mu.Unlock()
	if cleanupFailures != 0 || pending != 0 {
		return fmt.Errorf("Cloud Feature Flag cleanup retained an exact object or pending owner")
	}
	return nil
}

func featureFlagInventoryIdentity(environmentID string, key string) string {
	canonicalEnvironmentID, valid := client.CanonicalUUID(environmentID)
	if !valid {
		canonicalEnvironmentID = environmentID
	}
	return canonicalEnvironmentID + "\x00" + key
}

type cloudFeatureFlagProxy struct {
	target     url.URL
	server     *httptest.Server
	httpClient http.Client

	mu                sync.Mutex
	uiByPath          map[string]cloudSettingsFingerprint
	pendingByPath     map[string]cloudSettingsFingerprint
	nameUpdates       int
	preservedUpdates  int
	requestViolations int
}

func newCloudFeatureFlagProxy(target *url.URL) *cloudFeatureFlagProxy {
	proxy := &cloudFeatureFlagProxy{
		target: *target,
		httpClient: http.Client{
			Timeout: client.DefaultHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		uiByPath:      make(map[string]cloudSettingsFingerprint),
		pendingByPath: make(map[string]cloudSettingsFingerprint),
	}
	proxy.server = httptest.NewServer(http.HandlerFunc(proxy.handle))
	return proxy
}

func (p *cloudFeatureFlagProxy) apiOrigin() string {
	return p.server.URL
}

func (p *cloudFeatureFlagProxy) close() {
	p.server.Close()
}

func (p *cloudFeatureFlagProxy) handle(
	response http.ResponseWriter,
	request *http.Request,
) {
	if request == nil || request.URL == nil ||
		!strings.HasPrefix(request.URL.EscapedPath(), "/api/v1/") {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	requestBody, err := readCloudAcceptanceProxyBody(request.Body)
	if err != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	path := request.URL.EscapedPath()
	if request.Method == http.MethodPut && cloudFeatureFlagNamePath(path) {
		p.observeNameUpdate(path, requestBody)
	}

	target := p.target
	target.Path = request.URL.Path
	target.RawPath = request.URL.RawPath
	target.RawQuery = request.URL.RawQuery
	outbound, err := http.NewRequestWithContext(
		request.Context(),
		request.Method,
		target.String(),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	outbound.Header = request.Header.Clone()
	outbound.Host = target.Host
	upstream, err := p.httpClient.Do(outbound)
	if err != nil {
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	upstreamBody, readErr := readCloudAcceptanceProxyBody(upstream.Body)
	_ = upstream.Body.Close()
	if readErr != nil {
		p.recordViolation()
		writeCloudAcceptanceProxyFailure(response)
		return
	}
	if request.Method == http.MethodGet && cloudFeatureFlagExactPath(path) &&
		upstream.StatusCode >= http.StatusOK && upstream.StatusCode < http.StatusMultipleChoices {
		p.observeExactRead(path, upstreamBody)
	}
	for name, values := range upstream.Header {
		for _, value := range values {
			response.Header().Add(name, value)
		}
	}
	response.WriteHeader(upstream.StatusCode)
	_, _ = response.Write(upstreamBody)
}

func (p *cloudFeatureFlagProxy) observeNameUpdate(path string, body []byte) {
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil || len(payload) != 1 {
		p.recordViolation()
		return
	}
	var name string
	if json.Unmarshal(payload["name"], &name) != nil || name == "" {
		p.recordViolation()
		return
	}
	exactPath := strings.TrimSuffix(path, "/name")
	p.mu.Lock()
	baseline, found := p.uiByPath[exactPath]
	if !found {
		p.requestViolations++
	} else {
		p.pendingByPath[exactPath] = baseline
		p.nameUpdates++
	}
	p.mu.Unlock()
}

func (p *cloudFeatureFlagProxy) observeExactRead(path string, body []byte) {
	fingerprint, err := fingerprintCloudFeatureFlagUI(body)
	if err != nil {
		p.recordViolation()
		return
	}
	p.mu.Lock()
	if baseline, pending := p.pendingByPath[path]; pending {
		if cloudSettingsFingerprintDifference(baseline, fingerprint) != "" {
			p.requestViolations++
		} else {
			p.preservedUpdates++
		}
		delete(p.pendingByPath, path)
	}
	p.uiByPath[path] = fingerprint
	p.mu.Unlock()
}

func (p *cloudFeatureFlagProxy) recordViolation() {
	p.mu.Lock()
	p.requestViolations++
	p.mu.Unlock()
}

func (p *cloudFeatureFlagProxy) verifyUIPreservation(want int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.requestViolations != 0 || p.nameUpdates != want ||
		p.preservedUpdates != want || len(p.pendingByPath) != 0 {
		return fmt.Errorf(
			"Cloud Feature Flag UI preservation proof count mismatch (updates=%d, preserved=%d, pending=%d, violations=%d)",
			p.nameUpdates,
			p.preservedUpdates,
			len(p.pendingByPath),
			p.requestViolations,
		)
	}
	return nil
}

func cloudFeatureFlagExactPath(path string) bool {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	return len(segments) == 6 && segments[0] == "api" && segments[1] == "v1" &&
		segments[2] == "envs" && validUUID(segments[3]) &&
		segments[4] == "feature-flags" && segments[5] != ""
}

func cloudFeatureFlagNamePath(path string) bool {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	return len(segments) == 7 && segments[0] == "api" && segments[1] == "v1" &&
		segments[2] == "envs" && validUUID(segments[3]) &&
		segments[4] == "feature-flags" && segments[5] != "" && segments[6] == "name"
}

func fingerprintCloudFeatureFlagUI(body []byte) (cloudSettingsFingerprint, error) {
	var envelope struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success || envelope.Data == nil {
		return nil, fmt.Errorf("Cloud Feature Flag UI snapshot was unavailable")
	}
	required := []string{"isEnabled"}
	optional := []string{
		"disabledVariationId",
		"enabledVariationId",
		"targetUsers",
		"rules",
		"fallthrough",
		"exptIncludeAllTargets",
		"tags",
		"isArchived",
	}
	selected := make(map[string]any, len(required)+len(optional))
	for _, field := range required {
		raw, found := envelope.Data[field]
		if !found {
			return nil, fmt.Errorf("Cloud Feature Flag UI snapshot omitted a required field")
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return nil, fmt.Errorf("Cloud Feature Flag UI snapshot contained invalid JSON")
		}
		selected[field] = value
	}
	for _, field := range optional {
		raw, found := envelope.Data[field]
		if !found {
			continue
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return nil, fmt.Errorf("Cloud Feature Flag UI snapshot contained invalid JSON")
		}
		selected[field] = value
	}
	fingerprint := make(cloudSettingsFingerprint)
	if err := addCloudSettingsFingerprint(fingerprint, "$", selected); err != nil {
		return nil, fmt.Errorf("Cloud Feature Flag UI snapshot could not be fingerprinted")
	}
	return fingerprint, nil
}
