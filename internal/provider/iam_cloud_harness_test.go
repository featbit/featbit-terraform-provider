// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	testingterraform "github.com/hashicorp/terraform-plugin-testing/terraform"
)

type cloudIAMInventory struct {
	api      *client.Client
	children *cloudSegmentInventory

	memberID       string
	baselineDirect []string
	ownerBaseline  canonicalPolicy
	ownerPolicyKey string

	basePolicyKey   string
	scopedPolicyKey string
	adminGroupName  string
	developerName   string
}

func newCloudIAMInventory(
	apiClient *client.Client,
	children *cloudSegmentInventory,
	memberID string,
	baselineDirect []string,
	owner client.Policy,
	ownerPolicyKey string,
	basePolicyKey string,
	scopedPolicyKey string,
	adminGroupName string,
	developerName string,
) (*cloudIAMInventory, error) {
	canonicalMemberID, valid := client.CanonicalUUID(memberID)
	if !valid {
		return nil, errors.New("Cloud IAM inventory Member identity was invalid")
	}
	canonicalOwner, err := canonicalizeRemoteObservedPolicy(owner)
	if err != nil || canonicalOwner.Type != client.PolicyTypeSysManaged {
		return nil, errors.New("Cloud IAM inventory Owner baseline was invalid")
	}
	canonicalBaseline, valid := canonicalCloudIAMIDs(baselineDirect)
	if !valid {
		return nil, errors.New("Cloud IAM inventory direct-Policy baseline was invalid")
	}
	return &cloudIAMInventory{
		api:             apiClient,
		children:        children,
		memberID:        canonicalMemberID,
		baselineDirect:  canonicalBaseline,
		ownerBaseline:   canonicalOwner,
		ownerPolicyKey:  ownerPolicyKey,
		basePolicyKey:   basePolicyKey,
		scopedPolicyKey: scopedPolicyKey,
		adminGroupName:  adminGroupName,
		developerName:   developerName,
	}, nil
}

func (i *cloudIAMInventory) cleanupAndVerify(ctx context.Context) error {
	failures := 0
	owner, ownerFound, ownerErr := i.api.GetPolicyByKey(ctx, i.ownerPolicyKey)
	if ownerErr != nil || !ownerFound {
		failures++
	}
	base, baseFound, baseErr := i.api.GetPolicyByKey(ctx, i.basePolicyKey)
	if baseErr != nil {
		failures++
	}
	scoped, scopedFound, scopedErr := i.api.GetPolicyByKey(ctx, i.scopedPolicyKey)
	if scopedErr != nil {
		failures++
	}
	admin, adminFound, adminErr := i.api.GetGroupByName(ctx, i.adminGroupName)
	if adminErr != nil {
		failures++
	}
	developer, developerFound, developerErr := i.api.GetGroupByName(ctx, i.developerName)
	if developerErr != nil {
		failures++
	}

	if adminFound && ownerFound &&
		i.removeGroupPolicyIfPresent(ctx, admin.ID, owner.ID) != nil {
		failures++
	}
	if developerFound && baseFound &&
		i.removeGroupPolicyIfPresent(ctx, developer.ID, base.ID) != nil {
		failures++
	}
	if developerFound && scopedFound &&
		i.removeGroupPolicyIfPresent(ctx, developer.ID, scoped.ID) != nil {
		failures++
	}
	if developerFound &&
		i.removeGroupMemberIfPresent(ctx, developer.ID, i.memberID) != nil {
		failures++
	}
	if i.restoreDirectPolicies(ctx) != nil {
		failures++
	}

	for _, entry := range []struct {
		group client.Group
		found bool
	}{
		{group: admin, found: adminFound},
		{group: developer, found: developerFound},
	} {
		if !entry.found {
			continue
		}
		members, memberErr := i.api.ListGroupMemberIDs(ctx, entry.group.ID)
		policies, policyErr := i.api.ListGroupPolicyIDs(ctx, entry.group.ID)
		if memberErr != nil || policyErr != nil || len(members) != 0 || len(policies) != 0 {
			failures++
			continue
		}
		if deleteErr := i.api.DeleteGroup(ctx, entry.group.ID); deleteErr != nil {
			failures++
		}
	}

	for _, entry := range []struct {
		policy client.Policy
		found  bool
	}{
		{policy: base, found: baseFound},
		{policy: scoped, found: scopedFound},
	} {
		if !entry.found {
			continue
		}
		groups, groupErr := i.api.CountPolicyGroups(ctx, entry.policy.ID)
		members, memberErr := i.api.CountPolicyMembers(ctx, entry.policy.ID)
		if groupErr != nil || memberErr != nil || groups != 0 || members != 0 {
			failures++
			continue
		}
		if deleteErr := i.api.DeletePolicy(ctx, entry.policy.ID); deleteErr != nil {
			failures++
		}
	}

	if _, found, err := i.api.GetGroupByName(ctx, i.adminGroupName); err != nil || found {
		failures++
	}
	if _, found, err := i.api.GetGroupByName(ctx, i.developerName); err != nil || found {
		failures++
	}
	if _, found, err := i.api.GetPolicyByKey(ctx, i.basePolicyKey); err != nil || found {
		failures++
	}
	if _, found, err := i.api.GetPolicyByKey(ctx, i.scopedPolicyKey); err != nil || found {
		failures++
	}
	if ownerFound {
		canonicalOwner, err := canonicalizeRemoteObservedPolicy(owner)
		if err != nil || !samePolicyDefinition(canonicalOwner, i.ownerBaseline) {
			failures++
		}
	}
	if current, err := i.api.ListMemberDirectPolicyIDs(ctx, i.memberID); err != nil ||
		!slices.Equal(current, i.baselineDirect) {
		failures++
	}
	if i.children != nil && i.children.cleanupAndVerify(ctx) != nil {
		failures++
	}
	if failures != 0 {
		return fmt.Errorf("Cloud IAM cleanup retained or changed %d owned boundaries", failures)
	}
	return nil
}

func (i *cloudIAMInventory) removeGroupPolicyIfPresent(
	ctx context.Context,
	groupID string,
	policyID string,
) error {
	current, err := i.api.ListGroupPolicyIDs(ctx, groupID)
	if err != nil || !slices.Contains(current, policyID) {
		return err
	}
	if err := i.api.RemoveGroupPolicy(ctx, groupID, policyID); err != nil {
		return err
	}
	confirmed, err := i.api.ListGroupPolicyIDs(ctx, groupID)
	if err != nil || slices.Contains(confirmed, policyID) {
		return errors.New("Cloud IAM Group-Policy cleanup was not confirmed")
	}
	return nil
}

func (i *cloudIAMInventory) removeGroupMemberIfPresent(
	ctx context.Context,
	groupID string,
	memberID string,
) error {
	current, err := i.api.ListGroupMemberIDs(ctx, groupID)
	if err != nil || !slices.Contains(current, memberID) {
		return err
	}
	if err := i.api.RemoveGroupMember(ctx, groupID, memberID); err != nil {
		return err
	}
	confirmed, err := i.api.ListGroupMemberIDs(ctx, groupID)
	if err != nil || slices.Contains(confirmed, memberID) {
		return errors.New("Cloud IAM Group-Member cleanup was not confirmed")
	}
	return nil
}

func (i *cloudIAMInventory) restoreDirectPolicies(ctx context.Context) error {
	current, err := i.api.ListMemberDirectPolicyIDs(ctx, i.memberID)
	if err != nil {
		return err
	}
	for _, policyID := range i.baselineDirect {
		if slices.Contains(current, policyID) {
			continue
		}
		if err := i.api.AddMemberDirectPolicy(ctx, i.memberID, policyID); err != nil {
			return err
		}
		current, err = i.api.ListMemberDirectPolicyIDs(ctx, i.memberID)
		if err != nil || !slices.Contains(current, policyID) {
			return errors.New("Cloud IAM direct-Policy restoration was not confirmed")
		}
	}
	for _, policyID := range append([]string(nil), current...) {
		if slices.Contains(i.baselineDirect, policyID) {
			continue
		}
		if err := i.api.RemoveMemberDirectPolicy(ctx, i.memberID, policyID); err != nil {
			return err
		}
		current, err = i.api.ListMemberDirectPolicyIDs(ctx, i.memberID)
		if err != nil || slices.Contains(current, policyID) {
			return errors.New("Cloud IAM unexpected direct Policy was not removed")
		}
	}
	if !slices.Equal(current, i.baselineDirect) {
		return errors.New("Cloud IAM direct-Policy baseline was not restored")
	}
	return nil
}

func canonicalCloudIAMIDs(ids []string) ([]string, bool) {
	canonical := make([]string, 0, len(ids))
	for _, id := range ids {
		value, valid := client.CanonicalUUID(id)
		if !valid || slices.Contains(canonical, value) {
			return nil, false
		}
		canonical = append(canonical, value)
	}
	slices.Sort(canonical)
	return canonical, true
}

type cloudIAMRuntime struct {
	projectID       string
	devEnvironment  string
	prodEnvironment string
	devFlagID       string
	prodAllowedID   string
	prodDeniedID    string
	segmentID       string
	basePolicyID    string
	scopedPolicyID  string
	adminGroupID    string
	developerID     string
}

func (r *cloudIAMRuntime) complete() bool {
	for _, value := range []string{
		r.projectID,
		r.devEnvironment,
		r.prodEnvironment,
		r.devFlagID,
		r.prodAllowedID,
		r.prodDeniedID,
		r.segmentID,
		r.basePolicyID,
		r.scopedPolicyID,
		r.adminGroupID,
		r.developerID,
	} {
		if !client.ValidUUID(value) {
			return false
		}
	}
	return true
}

type cloudIAMEffectiveDefinitions struct {
	projectKey         string
	devFlagKey         string
	devFlagName        string
	prodAllowedKey     string
	prodAllowedName    string
	prodDeniedKey      string
	prodDeniedName     string
	segmentDescription string
}

func cloudIAMWaitProjectVisibility(
	ctx context.Context,
	apiClient *client.Client,
	projectKey string,
	wantVisible bool,
) error {
	for {
		projects, err := apiClient.ListProjects(ctx)
		visible := false
		if err == nil {
			for _, project := range projects {
				if project.Key == projectKey {
					visible = true
					break
				}
			}
		} else if !cloudIAMAuthorizationError(err) || wantVisible {
			return err
		}
		if visible == wantVisible {
			return nil
		}
		if !cloudObservationDelay(ctx) {
			return errors.New("Cloud IAM effective Project visibility did not converge")
		}
	}
}

func cloudIAMAuthorizationError(err error) bool {
	var apiError *client.APIError
	return errors.As(err, &apiError) &&
		apiError.Classification() == client.ClassificationAuthorization &&
		apiError.StatusCode() == 403
}

func cloudIAMVerifyEffectiveAccess(
	ctx context.Context,
	admin *client.Client,
	restricted *client.Client,
	runtime *cloudIAMRuntime,
	definition cloudIAMEffectiveDefinitions,
) error {
	if !runtime.complete() {
		return errors.New("Cloud IAM effective-access identities were incomplete")
	}
	if err := cloudIAMWaitProjectVisibility(ctx, restricted, definition.projectKey, true); err != nil {
		return errors.New("Cloud IAM restricted credential did not gain exact Project visibility")
	}
	if _, found, err := restricted.GetProject(ctx, runtime.projectID); err != nil || !found {
		return errors.New("Cloud IAM restricted credential could not read the exact Project")
	}
	if _, found, err := restricted.GetEnvironment(
		ctx,
		runtime.projectID,
		runtime.devEnvironment,
	); err != nil || !found {
		return errors.New("Cloud IAM restricted credential could not read the exact dev Environment")
	}
	if _, found, err := restricted.GetEnvironment(
		ctx,
		runtime.projectID,
		runtime.prodEnvironment,
	); err != nil || !found {
		return errors.New("Cloud IAM restricted credential could not read the exact prod Environment")
	}

	if err := cloudIAMRoundTripFlagName(
		ctx,
		admin,
		restricted,
		runtime.devEnvironment,
		definition.devFlagKey,
		runtime.devFlagID,
		definition.devFlagName,
		definition.devFlagName+" Effective Access",
	); err != nil {
		return fmt.Errorf(
			"Cloud IAM dev Feature Flag metadata permission did not round-trip: %w",
			err,
		)
	}
	if err := cloudIAMRoundTripSegmentDescription(
		ctx,
		admin,
		restricted,
		runtime.devEnvironment,
		runtime.segmentID,
		definition.segmentDescription,
		definition.segmentDescription+" effective access",
	); err != nil {
		return fmt.Errorf(
			"Cloud IAM dev Segment metadata permission did not round-trip: %w",
			err,
		)
	}
	if err := cloudIAMRoundTripFlagName(
		ctx,
		admin,
		restricted,
		runtime.prodEnvironment,
		definition.prodAllowedKey,
		runtime.prodAllowedID,
		definition.prodAllowedName,
		definition.prodAllowedName+" Effective Access",
	); err != nil {
		return fmt.Errorf(
			"Cloud IAM exact prod Feature Flag permission did not round-trip: %w",
			err,
		)
	}
	if err := restricted.UpdateFeatureFlagName(
		ctx,
		runtime.prodEnvironment,
		definition.prodDeniedKey,
		runtime.prodDeniedID,
		client.UpdateFeatureFlagNameRequest{Name: definition.prodDeniedName + " Unauthorized"},
	); !cloudIAMAuthorizationError(err) {
		_ = admin.UpdateFeatureFlagName(
			ctx,
			runtime.prodEnvironment,
			definition.prodDeniedKey,
			runtime.prodDeniedID,
			client.UpdateFeatureFlagNameRequest{Name: definition.prodDeniedName},
		)
		return errors.New("Cloud IAM unselected prod Feature Flag operation did not return 403")
	}
	return nil
}

func cloudIAMVerifyDetachedAccess(
	ctx context.Context,
	admin *client.Client,
	restricted *client.Client,
	runtime *cloudIAMRuntime,
	definition cloudIAMEffectiveDefinitions,
) error {
	if err := cloudIAMWaitProjectVisibility(ctx, restricted, definition.projectKey, false); err != nil {
		return errors.New("Cloud IAM restricted credential retained Project visibility without its Member binding")
	}
	err := restricted.UpdateFeatureFlagName(
		ctx,
		runtime.devEnvironment,
		definition.devFlagKey,
		runtime.devFlagID,
		client.UpdateFeatureFlagNameRequest{Name: definition.devFlagName + " Detached"},
	)
	if !cloudIAMAuthorizationError(err) {
		_ = admin.UpdateFeatureFlagName(
			ctx,
			runtime.devEnvironment,
			definition.devFlagKey,
			runtime.devFlagID,
			client.UpdateFeatureFlagNameRequest{Name: definition.devFlagName},
		)
		return errors.New("Cloud IAM detached Member credential did not return 403")
	}
	return nil
}

func cloudIAMRoundTripFlagName(
	ctx context.Context,
	admin *client.Client,
	restricted *client.Client,
	environmentID string,
	key string,
	flagID string,
	original string,
	temporary string,
) error {
	restore := func() {
		_ = admin.UpdateFeatureFlagName(
			ctx,
			environmentID,
			key,
			flagID,
			client.UpdateFeatureFlagNameRequest{Name: original},
		)
	}
	mutationErr := restricted.UpdateFeatureFlagName(
		ctx,
		environmentID,
		key,
		flagID,
		client.UpdateFeatureFlagNameRequest{Name: temporary},
	)
	if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
		return mutationErr
	}
	if !cloudIAMWaitFlagName(ctx, admin, environmentID, key, flagID, temporary) {
		restore()
		return errors.New("Cloud IAM Feature Flag metadata mutation was not observed")
	}
	mutationErr = restricted.UpdateFeatureFlagName(
		ctx,
		environmentID,
		key,
		flagID,
		client.UpdateFeatureFlagNameRequest{Name: original},
	)
	if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
		restore()
		return mutationErr
	}
	if !cloudIAMWaitFlagName(ctx, admin, environmentID, key, flagID, original) {
		restore()
		return errors.New("Cloud IAM Feature Flag metadata restoration was not observed")
	}
	return nil
}

func cloudIAMWaitFlagName(
	ctx context.Context,
	apiClient *client.Client,
	environmentID string,
	key string,
	flagID string,
	want string,
) bool {
	for attempt := 0; attempt < 30; attempt++ {
		flag, status, err := apiClient.GetFeatureFlag(ctx, environmentID, key)
		if err == nil && status == client.FeatureFlagStatusActive &&
			client.EqualUUID(flag.ID, flagID) && flag.Name == want {
			return true
		}
		if !cloudObservationDelay(ctx) {
			break
		}
	}
	return false
}

func cloudIAMRoundTripSegmentDescription(
	ctx context.Context,
	admin *client.Client,
	restricted *client.Client,
	environmentID string,
	segmentID string,
	original string,
	temporary string,
) error {
	restore := func() {
		_ = admin.UpdateSegmentDescription(
			ctx,
			environmentID,
			segmentID,
			client.UpdateSegmentDescriptionRequest{Description: original},
		)
	}
	mutationErr := restricted.UpdateSegmentDescription(
		ctx,
		environmentID,
		segmentID,
		client.UpdateSegmentDescriptionRequest{Description: temporary},
	)
	if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
		return mutationErr
	}
	if !cloudWaitExactSegment(
		ctx,
		admin,
		environmentID,
		segmentID,
		func(segment client.Segment) bool { return segment.Description == temporary },
	) {
		restore()
		return errors.New("Cloud IAM Segment metadata mutation was not observed")
	}
	mutationErr = restricted.UpdateSegmentDescription(
		ctx,
		environmentID,
		segmentID,
		client.UpdateSegmentDescriptionRequest{Description: original},
	)
	if mutationErr != nil && !mutationNeedsReconciliation(mutationErr) {
		restore()
		return mutationErr
	}
	if !cloudWaitExactSegment(
		ctx,
		admin,
		environmentID,
		segmentID,
		func(segment client.Segment) bool { return segment.Description == original },
	) {
		restore()
		return errors.New("Cloud IAM Segment metadata restoration was not observed")
	}
	return nil
}

func cloudIAMStateValue(state *testingterraform.State, address string, attribute string) (string, bool) {
	resourceState, found := state.RootModule().Resources[address]
	if !found || resourceState.Primary == nil {
		return "", false
	}
	value, found := resourceState.Primary.Attributes[attribute]
	return value, found
}

func TestCloudIAMInventoryRestoresMemberAndCleansOnlyOwnedGraph(t *testing.T) {
	fixture := newIAMWorkflowFixture(t)
	defer fixture.close()

	fixture.mu.Lock()
	fixture.policies[iamWorkflowBasePolicyID] = iamWorkflowBasePolicy()
	fixture.policies[iamWorkflowScopedPolicyID] = iamWorkflowScopedPolicy()
	fixture.groups[iamWorkflowAdminGroupID] = client.Group{
		ID: iamWorkflowAdminGroupID, Name: "IAM Administrators", Description: "Owner access",
	}
	fixture.groups[iamWorkflowDeveloperID] = client.Group{
		ID: iamWorkflowDeveloperID, Name: "IAM Developers", Description: "Scoped developer access",
	}
	fixture.groupPolicies[iamWorkflowAdminGroupID] = map[string]struct{}{
		iamWorkflowOwnerPolicyID: {},
	}
	fixture.groupPolicies[iamWorkflowDeveloperID] = map[string]struct{}{
		iamWorkflowBasePolicyID: {}, iamWorkflowScopedPolicyID: {},
	}
	fixture.groupMembers[iamWorkflowAdminGroupID] = make(map[string]struct{})
	fixture.groupMembers[iamWorkflowDeveloperID] = map[string]struct{}{
		iamWorkflowMemberID: {},
	}
	fixture.directPolicyIDs = make(map[string]struct{})
	owner := cloneProviderPolicy(fixture.ownerBaseline)
	fixture.mu.Unlock()

	apiClient := newCloudIAMClient(
		t,
		fixture.apiOrigin(),
		syntheticProviderAccessToken,
		"protocol-test",
	)
	inventory, err := newCloudIAMInventory(
		apiClient,
		nil,
		iamWorkflowMemberID,
		[]string{iamWorkflowOwnerPolicyID},
		owner,
		"Owner",
		"iam-base-access",
		"iam-scoped-access",
		"IAM Administrators",
		"IAM Developers",
	)
	if err != nil {
		t.Fatalf("newCloudIAMInventory() error = %v", err)
	}
	if err := inventory.cleanupAndVerify(context.Background()); err != nil {
		t.Fatalf("cleanupAndVerify() error = %v", err)
	}

	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.policies) != 1 || len(fixture.groups) != 0 ||
		len(fixture.groupPolicies) != 0 || len(fixture.groupMembers) != 0 {
		t.Fatal("Cloud IAM cleanup retained a test-owned Policy, Group, or relationship")
	}
	if len(fixture.directPolicyIDs) != 1 {
		t.Fatal("Cloud IAM cleanup did not restore the direct-Policy baseline")
	}
	if _, found := fixture.directPolicyIDs[iamWorkflowOwnerPolicyID]; !found {
		t.Fatal("Cloud IAM cleanup restored the wrong direct Policy")
	}
	if len(fixture.violations) != 0 {
		t.Fatal("Cloud IAM cleanup escaped the documented request contract")
	}
}
