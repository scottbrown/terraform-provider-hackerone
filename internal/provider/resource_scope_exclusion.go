// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

var (
	_ resource.Resource                = &scopeExclusionResource{}
	_ resource.ResourceWithConfigure   = &scopeExclusionResource{}
	_ resource.ResourceWithImportState = &scopeExclusionResource{}
)

// NewScopeExclusionResource manages an out-of-scope entry for a program.
func NewScopeExclusionResource() resource.Resource { return &scopeExclusionResource{} }

type scopeExclusionResource struct {
	client *client.Client
}

type scopeExclusionModel struct {
	ID        types.String `tfsdk:"id"`
	ProgramID types.String `tfsdk:"program_id"`
	Category  types.String `tfsdk:"category"`
	Details   types.String `tfsdk:"details"`
}

func (r *scopeExclusionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scope_exclusion"
}

func (r *scopeExclusionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an out-of-scope entry (scope exclusion) for a HackerOne program.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID in the form `program_id/exclusion_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"program_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the program.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"category": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Asset category of the exclusion (e.g. `url`, `cidr`, `apple_store_app_id`).",
			},
			"details": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The identifier being excluded (e.g. the URL or CIDR).",
			},
		},
	}
}

func (r *scopeExclusionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		r.client = c
	}
}

func (r *scopeExclusionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scopeExclusionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, attrs, err := r.client.CreateScopeExclusion(ctx, plan.ProgramID.ValueString(), client.ScopeExclusionAttributes{
		Category: plan.Category.ValueString(),
		Details:  plan.Details.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating scope exclusion", err.Error())
		return
	}
	plan.ID = types.StringValue(compositeID(plan.ProgramID.ValueString(), id))
	plan.Category = types.StringValue(attrs.Category)
	plan.Details = types.StringValue(attrs.Details)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scopeExclusionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scopeExclusionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	programID, exclusionID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid scope exclusion ID", err.Error())
		return
	}
	all, err := r.client.GetScopeExclusions(ctx, programID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading scope exclusions", err.Error())
		return
	}
	attrs, ok := all[exclusionID]
	if !ok {
		// The exclusion is gone remotely.
		resp.State.RemoveResource(ctx)
		return
	}
	state.ProgramID = types.StringValue(programID)
	state.Category = types.StringValue(attrs.Category)
	state.Details = types.StringValue(attrs.Details)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *scopeExclusionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state scopeExclusionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	programID, exclusionID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid scope exclusion ID", err.Error())
		return
	}
	attrs, err := r.client.UpdateScopeExclusion(ctx, programID, exclusionID, client.ScopeExclusionAttributes{
		Category: plan.Category.ValueString(),
		Details:  plan.Details.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating scope exclusion", err.Error())
		return
	}
	plan.ID = state.ID
	plan.Category = types.StringValue(attrs.Category)
	plan.Details = types.StringValue(attrs.Details)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *scopeExclusionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scopeExclusionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	programID, exclusionID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid scope exclusion ID", err.Error())
		return
	}
	if err := r.client.DeleteScopeExclusion(ctx, programID, exclusionID); err != nil {
		if client.NotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting scope exclusion", err.Error())
	}
}

func (r *scopeExclusionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID is the composite program_id/exclusion_id.
	if _, _, err := splitCompositeID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format program_id/exclusion_id, got: "+req.ID)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func compositeID(a, b string) string { return a + "/" + b }

func splitCompositeID(id string) (string, string, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected composite ID program_id/exclusion_id, got %q", id)
	}
	return parts[0], parts[1], nil
}
