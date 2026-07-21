// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

var (
	_ resource.Resource                = &assetScopeResource{}
	_ resource.ResourceWithConfigure   = &assetScopeResource{}
	_ resource.ResourceWithImportState = &assetScopeResource{}
)

// NewAssetScopeResource binds an organization asset into a program's scope
// (the "in scope" list). This is the write path HackerOne actually exposes:
// POST /organizations/{org_id}/assets/{asset_id}/scopes with a programs
// relationship. The program's structured_scopes list is the read side.
func NewAssetScopeResource() resource.Resource { return &assetScopeResource{} }

type assetScopeResource struct {
	client *client.Client
}

type assetScopeModel struct {
	ID                    types.String `tfsdk:"id"`
	OrganizationID        types.String `tfsdk:"organization_id"`
	AssetID               types.String `tfsdk:"asset_id"`
	ProgramID             types.String `tfsdk:"program_id"`
	ScopeID               types.String `tfsdk:"scope_id"`
	EligibleForSubmission types.Bool   `tfsdk:"eligible_for_submission"`
	EligibleForBounty     types.Bool   `tfsdk:"eligible_for_bounty"`
	Instruction           types.String `tfsdk:"instruction"`
}

func (r *assetScopeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_asset_scope"
}

func (r *assetScopeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Places an organization asset into a program's scope (the 'in scope' list), " +
			"controlling submission and bounty eligibility.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID `organization_id/asset_id/program_id/scope_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the organization that owns the asset.",
				PlanModifiers:       replace,
			},
			"asset_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the asset to place in scope.",
				PlanModifiers:       replace,
			},
			"program_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the program whose scope the asset joins.",
				PlanModifiers:       replace,
			},
			"scope_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The structured-scope ID created by the binding (as reported in the program's structured_scopes).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"eligible_for_submission": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether reports may be submitted against this asset. Defaults to `true`.",
			},
			"eligible_for_bounty": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether this asset is eligible for bounties. Defaults to `false`.",
			},
			"instruction": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Guidance shown to hackers for this scope entry.",
			},
		},
	}
}

func (r *assetScopeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		r.client = c
	}
}

func (r *assetScopeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan assetScopeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID := plan.OrganizationID.ValueString()
	assetID := plan.AssetID.ValueString()
	programID := plan.ProgramID.ValueString()

	scopeID, attrs, err := r.client.AddAssetToScope(ctx, orgID, assetID, programID, r.attrsFromPlan(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error adding asset to scope", err.Error())
		return
	}
	plan.ScopeID = types.StringValue(scopeID)
	plan.ID = types.StringValue(assetScopeID(orgID, assetID, programID, scopeID))
	r.applyWriteAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *assetScopeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state assetScopeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, programID, scopeID, err := splitAssetScopeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset scope ID", err.Error())
		return
	}
	scope, err := r.client.GetStructuredScope(ctx, programID, scopeID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading structured scope", err.Error())
		return
	}
	if scope == nil {
		// No longer present in the program's scope list (removed or archived).
		resp.State.RemoveResource(ctx)
		return
	}
	state.OrganizationID = types.StringValue(orgID)
	state.AssetID = types.StringValue(assetID)
	state.ProgramID = types.StringValue(programID)
	state.ScopeID = types.StringValue(scopeID)
	state.EligibleForSubmission = types.BoolValue(scope.EligibleForSubmission)
	state.EligibleForBounty = types.BoolValue(scope.EligibleForBounty)
	state.Instruction = optionalString(scope.Instruction)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *assetScopeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state assetScopeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, _, scopeID, err := splitAssetScopeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset scope ID", err.Error())
		return
	}
	attrs, err := r.client.UpdateAssetScope(ctx, orgID, assetID, scopeID, r.attrsFromPlan(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating asset scope", err.Error())
		return
	}
	plan.ID = state.ID
	plan.ScopeID = state.ScopeID
	r.applyWriteAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *assetScopeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state assetScopeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, programID, _, err := splitAssetScopeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset scope ID", err.Error())
		return
	}
	if err := r.client.ArchiveAssetScope(ctx, orgID, assetID, programID); err != nil {
		if client.NotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error removing asset from scope", err.Error())
	}
}

func (r *assetScopeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, _, _, _, err := splitAssetScopeID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID",
			"Expected format organization_id/asset_id/program_id/scope_id, got: "+req.ID)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// attrsFromPlan builds the write payload, populating both notify-field
// spellings (the API differs between create and update) with the same value.
func (r *assetScopeResource) attrsFromPlan(m assetScopeModel) client.AssetScopeAttributes {
	sub := m.EligibleForSubmission.ValueBool()
	bounty := m.EligibleForBounty.ValueBool()
	return client.AssetScopeAttributes{
		EligibleForSubmission: &sub,
		EligibleForBounty:     &bounty,
		Instruction:           m.Instruction.ValueString(),
	}
}

func (r *assetScopeResource) applyWriteAttrs(m *assetScopeModel, attrs *client.AssetScopeAttributes) {
	if attrs.EligibleForSubmission != nil {
		m.EligibleForSubmission = types.BoolValue(*attrs.EligibleForSubmission)
	}
	if attrs.EligibleForBounty != nil {
		m.EligibleForBounty = types.BoolValue(*attrs.EligibleForBounty)
	}
	m.Instruction = optionalString(attrs.Instruction)
}

func assetScopeID(org, asset, program, scope string) string {
	return strings.Join([]string{org, asset, program, scope}, "/")
}

func splitAssetScopeID(id string) (org, asset, program, scope string, err error) {
	parts := strings.Split(id, "/")
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("expected organization_id/asset_id/program_id/scope_id, got %q", id)
	}
	for _, p := range parts {
		if p == "" {
			return "", "", "", "", fmt.Errorf("empty segment in ID %q", id)
		}
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}
