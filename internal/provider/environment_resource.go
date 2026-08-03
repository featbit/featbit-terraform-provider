// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"sync"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*environmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*environmentResource)(nil)
	_ resource.ResourceWithImportState = (*environmentResource)(nil)
)

type environmentResource struct {
	client   *client.Client
	lockOnce sync.Once
	locks    *environmentLockManager
}

func newEnvironmentResource() resource.Resource {
	return &environmentResource{}
}

func (r *environmentResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FeatBit Environment through the documented public API.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable parent Project UUID.",
				Validators: []validator.String{
					uuidValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment UUID.",
				PlanModifiers: []planmodifier.String{
					useStateForUnknownIfUnchanged(
						path.Root("project_id"),
						path.Root("key"),
					),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Environment display name.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable Environment key within the parent Project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Environment description. An omitted value canonicalizes to an empty string.",
				Default:             stringdefault.StaticString(""),
			},
		},
	}
}

func (r *environmentResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *environmentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing an Environment", &resp.Diagnostics) {
		return
	}

	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := plan.ProjectID.ValueString()
	key := plan.Key.ValueString()

	project, found, err := r.client.GetProject(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Environments",
			"The provider could not read the exact parent Project for the scoped Environment create preflight. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Environment Parent Project Not Found",
			"The configured exact parent Project does not exist, so the Environment cannot be created.",
		)
		return
	}
	matchCount := countProjectEnvironmentsByKey(project.Environments, key)
	if matchCount != 0 {
		detail := "An Environment with the configured exact key already exists in the parent Project. Terraform will not adopt it automatically; import the intended Environment by its two UUIDs or choose another key."
		if matchCount > 1 {
			detail = "Multiple Environments have the configured exact key in the parent Project, so creation is ambiguous. Resolve the duplicates before retrying."
		}
		resp.Diagnostics.AddError("FeatBit Environment Create Preflight Failed", detail)
		return
	}

	created, err := r.client.CreateEnvironment(ctx, projectID, client.CreateEnvironmentRequest{
		Name:        plan.Name.ValueString(),
		Key:         key,
		Description: plan.Description.ValueString(),
	})
	if err != nil {
		if mutationOutcomeAmbiguous(err) {
			r.reconcileAmbiguousCreate(ctx, projectID, key, err, &resp.Diagnostics)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Create FeatBit Environment",
			"The Environment create request failed without a confirmed remote object. "+err.Error()+".",
		)
		return
	}

	provisional := flattenEnvironment(projectID, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &provisional)...)
	if resp.Diagnostics.HasError() {
		return
	}

	canonical, found, err := r.client.GetEnvironment(ctx, projectID, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Created FeatBit Environment",
			"The Environment was created, but its canonical state could not be confirmed. The confirmed identity remains in Terraform state so the operation can be recovered safely. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Created FeatBit Environment Is Unconfirmed",
			"The Environment create response supplied an identity, but the exact parent Project did not contain it. The identity remains in Terraform state for safe recovery.",
		)
		return
	}
	state := flattenEnvironment(projectID, canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *environmentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing an Environment", &resp.Diagnostics) {
		return
	}

	var projectID types.String
	var environmentID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("project_id"), &projectID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &environmentID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	environment, found, err := r.client.GetEnvironment(
		ctx,
		projectID.ValueString(),
		environmentID.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Environment",
			"The provider could not confirm the Environment through the documented public API. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state := flattenEnvironment(projectID.ValueString(), environment)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *environmentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing an Environment", &resp.Diagnostics) {
		return
	}

	var state environmentModel
	var plan environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := state.ProjectID.ValueString()
	environmentID := state.ID.ValueString()

	release, err := r.environmentLocks().acquire(ctx, projectID, environmentID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Acquire FeatBit Environment Update Lock",
			"The Environment update was canceled while waiting to preserve its current UI-owned settings. Terraform state has been preserved.",
		)
		return
	}
	defer release()

	current, found, err := r.client.GetEnvironment(ctx, projectID, environmentID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Environment Before Update",
			"The provider could not read the current Environment settings immediately before Update. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"FeatBit Environment Disappeared Before Update",
			"The exact Environment no longer exists. Terraform state has been preserved so refresh can reconcile the deletion safely.",
		)
		return
	}

	err = r.client.UpdateEnvironment(
		ctx,
		projectID,
		environmentID,
		current,
		client.UpdateEnvironmentRequest{
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueString(),
		},
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Update FeatBit Environment",
			"The Environment update did not complete. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}

	canonical, found, err := r.client.GetEnvironment(ctx, projectID, environmentID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Updated FeatBit Environment",
			"The Environment update response succeeded, but canonical state could not be confirmed. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Updated FeatBit Environment Is Unconfirmed",
			"The Environment update response succeeded, but exact absence was reported during the canonical read. Terraform state has been preserved for recovery.",
		)
		return
	}
	canonicalState := flattenEnvironment(projectID, canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &canonicalState)...)
}

func (r *environmentResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing an Environment", &resp.Diagnostics) {
		return
	}

	var projectID types.String
	var environmentID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("project_id"), &projectID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &environmentID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteErr := r.client.DeleteEnvironment(
		ctx,
		projectID.ValueString(),
		environmentID.ValueString(),
	)
	_, found, readErr := r.client.GetEnvironment(
		ctx,
		projectID.ValueString(),
		environmentID.ValueString(),
	)
	if readErr != nil {
		detail := "The provider could not prove exact Environment absence after the delete attempt. Terraform state has been preserved. " + readErr.Error() + "."
		if deleteErr != nil {
			detail = "The Environment delete request failed and exact parent-scoped absence could not be proven. Terraform state has been preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Environment Deletion", detail)
		return
	}
	if found {
		detail := "The Environment still exists after the delete request. Terraform state has been preserved."
		if deleteErr != nil {
			detail = "The Environment delete request failed and the exact Environment still exists. Terraform state has been preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("FeatBit Environment Was Not Deleted", detail)
		return
	}

	resp.State.RemoveResource(ctx)
}

func (r *environmentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	components := strings.Split(req.ID, "/")
	if len(components) != 2 || !validUUID(components[0]) || !validUUID(components[1]) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Environment Import Identifier",
			"Import an Environment as <project_uuid>/<environment_uuid>, with both values in 8-4-4-4-12 hexadecimal UUID form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), components[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), components[1])...)
}

func (r *environmentResource) reconcileAmbiguousCreate(
	ctx context.Context,
	projectID string,
	key string,
	createErr error,
	diagnostics *diag.Diagnostics,
) {
	project, found, readErr := r.client.GetProject(ctx, projectID)
	if readErr != nil {
		diagnostics.AddError(
			"FeatBit Environment Create Outcome Is Unconfirmed",
			"The create result was ambiguous and the exact parent Project could not be read. Terraform did not retry or adopt any object. Verify the remote system before retrying, then import the intended Environment by its two UUIDs if it exists. "+createErr.Error()+".",
		)
		return
	}
	matchCount := 0
	if found {
		matchCount = countProjectEnvironmentsByKey(project.Environments, key)
	}
	switch matchCount {
	case 0:
		diagnostics.AddError(
			"Unable to Create FeatBit Environment",
			"The create result was ambiguous, but the exact parent Project contains no Environment with the configured key. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	case 1:
		diagnostics.AddError(
			"FeatBit Environment Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one Environment now has the configured key in the parent Project. Terraform did not retry or adopt it. Verify that object, then import it by its two UUIDs or remove it before retrying.",
		)
	default:
		diagnostics.AddError(
			"FeatBit Environment Create Outcome Is Ambiguous",
			"The create result was ambiguous and multiple Environments now have the configured exact key in the parent Project. Terraform did not retry or adopt any object. Resolve the duplicates before continuing.",
		)
	}
}

func (r *environmentResource) environmentLocks() *environmentLockManager {
	r.lockOnce.Do(func() {
		if r.locks == nil {
			r.locks = newEnvironmentLockManager()
		}
	})
	return r.locks
}

func countProjectEnvironmentsByKey(environments []client.ProjectEnvironment, key string) int {
	count := 0
	for _, environment := range environments {
		if environment.Key == key {
			count++
		}
	}
	return count
}
