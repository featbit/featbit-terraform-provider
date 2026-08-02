// Copyright 2026 FeatBit
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"

	"github.com/featbit/terraform-provider-featbit/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
)

type projectResource struct {
	client *client.Client
}

func newProjectResource() resource.Resource {
	return &projectResource{}
}

func (r *projectResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a FeatBit Project through the documented public API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Project UUID.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Project display name.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Immutable Project key.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"environments": projectResourceEnvironmentsAttribute(),
		},
	}
}

func projectResourceEnvironmentsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Computed: true,
		MarkdownDescription: "Canonical, non-owning observations of the Project environments. " +
			"Manage additional environments with featbit_environment.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Environment UUID.",
				},
				"name": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Environment display name.",
				},
				"key": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Environment key.",
				},
				"description": schema.StringAttribute{
					Computed:            true,
					MarkdownDescription: "Environment description.",
				},
			},
		},
	}
}

func (r *projectResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	r.client = clientFromProviderData(req.ProviderData, "Resource", &resp.Diagnostics)
}

func (r *projectResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	if !requireAPIClient(r.client, "managing a Project", &resp.Diagnostics) {
		return
	}

	var name types.String
	var key types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("key"), &key)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projects, err := r.client.ListProjects(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Check Existing FeatBit Projects",
			"The provider could not complete the exact-key Project create preflight. "+err.Error()+".",
		)
		return
	}
	matchCount := countProjectsByKey(projects, key.ValueString())
	if matchCount != 0 {
		detail := "A Project with the configured exact key already exists. Terraform will not adopt it automatically; import the intended Project by UUID or choose another key."
		if matchCount > 1 {
			detail = "Multiple Projects have the configured exact key, so creation is ambiguous. Resolve the duplicates before retrying."
		}
		resp.Diagnostics.AddError("FeatBit Project Create Preflight Failed", detail)
		return
	}

	created, err := r.client.CreateProject(ctx, client.CreateProjectRequest{
		Name: name.ValueString(),
		Key:  key.ValueString(),
	})
	if err != nil {
		if projectMutationOutcomeAmbiguous(err) {
			r.reconcileAmbiguousCreate(ctx, key.ValueString(), err, &resp.Diagnostics)
			return
		}
		resp.Diagnostics.AddError(
			"Unable to Create FeatBit Project",
			"The Project create request failed without a confirmed remote object. "+err.Error()+".",
		)
		return
	}

	// Preserve the server-confirmed identity if the required canonical
	// read-after-write is temporarily unavailable.
	provisional := flattenProject(created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &provisional)...)
	if resp.Diagnostics.HasError() {
		return
	}

	canonical, found, err := r.client.GetProject(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Created FeatBit Project",
			"The Project was created, but its canonical state could not be confirmed. "+
				"The confirmed identity remains in Terraform state so the operation can be recovered safely. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Created FeatBit Project Is Unconfirmed",
			"The Project create response supplied an identity, but the complete Project collection did not contain it. The identity remains in Terraform state for safe recovery.",
		)
		return
	}
	state := flattenProject(canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Project", &resp.Diagnostics) {
		return
	}

	var projectID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &projectID)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project, found, err := r.client.GetProject(ctx, projectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read FeatBit Project",
			"The provider could not confirm the Project through the documented public API. "+
				"Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state := flattenProject(project)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Project", &resp.Diagnostics) {
		return
	}

	var projectID types.String
	var name types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &projectID)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UpdateProject(ctx, projectID.ValueString(), client.UpdateProjectRequest{
		Name: name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Update FeatBit Project",
			"The Project update did not complete. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}

	canonical, found, err := r.client.GetProject(ctx, projectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Confirm Updated FeatBit Project",
			"The Project update response succeeded, but canonical state could not be confirmed. Terraform state has been preserved. "+err.Error()+".",
		)
		return
	}
	if !found {
		resp.Diagnostics.AddError(
			"Updated FeatBit Project Is Unconfirmed",
			"The Project update response succeeded, but exact absence was reported during the canonical read. Terraform state has been preserved for recovery.",
		)
		return
	}
	state := flattenProject(canonical)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *projectResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	resp.State = req.State
	if !requireAPIClient(r.client, "managing a Project", &resp.Diagnostics) {
		return
	}

	var projectID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &projectID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteErr := r.client.DeleteProject(ctx, projectID.ValueString())
	_, found, readErr := r.client.GetProject(ctx, projectID.ValueString())
	if readErr != nil {
		detail := "The provider could not prove exact Project absence after the delete attempt. Terraform state has been preserved. " + readErr.Error() + "."
		if deleteErr != nil {
			detail = "The Project delete request failed and exact absence could not be proven. Terraform state has been preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("Unable to Confirm FeatBit Project Deletion", detail)
		return
	}
	if found {
		detail := "The Project still exists after the delete request. Terraform state has been preserved."
		if deleteErr != nil {
			detail = "The Project delete request failed and the exact Project still exists. Terraform state has been preserved. " + deleteErr.Error() + "."
		}
		resp.Diagnostics.AddError("FeatBit Project Was Not Deleted", detail)
		return
	}

	// Exact zero in the complete collection proves absence even when the
	// direct mutation result was ambiguous or the object was already absent.
	resp.State.RemoveResource(ctx)
}

func (r *projectResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if !validUUID(req.ID) {
		resp.Diagnostics.AddError(
			"Invalid FeatBit Project Import Identifier",
			"Import a Project with exactly one UUID in 8-4-4-4-12 hexadecimal form.",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *projectResource) reconcileAmbiguousCreate(
	ctx context.Context,
	key string,
	createErr error,
	diagnostics *diag.Diagnostics,
) {
	projects, listErr := r.client.ListProjects(ctx)
	if listErr != nil {
		diagnostics.AddError(
			"FeatBit Project Create Outcome Is Unconfirmed",
			"The create result was ambiguous and the complete Project collection could not be read. "+
				"Terraform did not retry or adopt any object. Verify the remote system before retrying, then import the intended Project by UUID if it exists. "+createErr.Error()+".",
		)
		return
	}

	switch countProjectsByKey(projects, key) {
	case 0:
		diagnostics.AddError(
			"Unable to Create FeatBit Project",
			"The create result was ambiguous, but the complete Project collection contains no exact-key match. Terraform did not retry the mutation. "+createErr.Error()+".",
		)
	case 1:
		diagnostics.AddError(
			"FeatBit Project Create Outcome Requires Recovery",
			"The create result was ambiguous and exactly one Project now has the configured key. Terraform did not retry or adopt it. Verify that object, then import it by UUID or remove it before retrying.",
		)
	default:
		diagnostics.AddError(
			"FeatBit Project Create Outcome Is Ambiguous",
			"The create result was ambiguous and multiple Projects now have the configured exact key. Terraform did not retry or adopt any object. Resolve the duplicates before continuing.",
		)
	}
}

func countProjectsByKey(projects []client.Project, key string) int {
	count := 0
	for _, project := range projects {
		if project.Key == key {
			count++
		}
	}
	return count
}

func projectMutationOutcomeAmbiguous(err error) bool {
	var apiError *client.APIError
	if !errors.As(err, &apiError) {
		return true
	}
	switch apiError.Classification() {
	case client.ClassificationRateLimited,
		client.ClassificationTransientServer,
		client.ClassificationTimeout,
		client.ClassificationCanceled,
		client.ClassificationNetwork,
		client.ClassificationAmbiguous:
		return true
	default:
		return false
	}
}
