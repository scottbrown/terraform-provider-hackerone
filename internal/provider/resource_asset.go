// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
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
	_ resource.Resource                = &assetResource{}
	_ resource.ResourceWithConfigure   = &assetResource{}
	_ resource.ResourceWithImportState = &assetResource{}
)

// NewAssetResource manages an organization asset. Assets are org-scoped
// (/organizations/{org_id}/assets); adding one to a program's scope is a
// separate operation not modeled by this resource.
func NewAssetResource() resource.Resource { return &assetResource{} }

type assetResource struct {
	client *client.Client
}

type assetModel struct {
	ID             types.String `tfsdk:"id"`
	AssetID        types.String `tfsdk:"asset_id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	AssetType      types.String `tfsdk:"asset_type"`
	Identifier     types.String `tfsdk:"identifier"`
	Description    types.String `tfsdk:"description"`
	MaxSeverity    types.String `tfsdk:"max_severity"`
	Reference      types.String `tfsdk:"reference"`
}

func (r *assetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_asset"
}

func (r *assetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an asset in a HackerOne organization's asset inventory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID in the form `organization_id/asset_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"asset_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The bare numeric asset ID (without the organization prefix). Reference this from `hackerone_asset_scope.asset_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the organization that owns the asset.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"asset_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Asset type (e.g. `domain`, `url`, `cidr`, `github_repo`). Immutable after creation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"identifier": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The asset identifier (e.g. `example.com`). Immutable after creation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Free-text description of the asset.",
			},
			"max_severity": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum severity for findings on this asset (`none`, `low`, `medium`, `high`, `critical`).",
			},
			"reference": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "An internal reference identifier for the asset.",
			},
		},
	}
}

func (r *assetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		r.client = c
	}
}

func (r *assetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan assetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID := plan.OrganizationID.ValueString()
	id, attrs, err := r.client.CreateAsset(ctx, orgID, client.AssetAttributes{
		AssetType:   plan.AssetType.ValueString(),
		Identifier:  plan.Identifier.ValueString(),
		Description: plan.Description.ValueString(),
		MaxSeverity: plan.MaxSeverity.ValueString(),
		Reference:   plan.Reference.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating asset", err.Error())
		return
	}
	plan.ID = types.StringValue(compositeID(orgID, id))
	plan.AssetID = types.StringValue(id)
	applyAssetAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *assetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state assetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset ID", err.Error())
		return
	}
	attrs, err := r.client.GetAsset(ctx, orgID, assetID)
	if err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading asset", err.Error())
		return
	}
	state.OrganizationID = types.StringValue(orgID)
	state.AssetID = types.StringValue(assetID)
	applyAssetAttrs(&state, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *assetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state assetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset ID", err.Error())
		return
	}
	attrs, err := r.client.UpdateAsset(ctx, orgID, assetID, client.AssetAttributes{
		Description: plan.Description.ValueString(),
		MaxSeverity: plan.MaxSeverity.ValueString(),
		Reference:   plan.Reference.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating asset", err.Error())
		return
	}
	plan.ID = state.ID
	plan.AssetID = state.AssetID
	applyAssetAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *assetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state assetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, assetID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid asset ID", err.Error())
		return
	}
	// HackerOne has no hard-delete for assets; archive is the destroy analog.
	if err := r.client.ArchiveAsset(ctx, orgID, assetID); err != nil {
		if client.NotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error archiving asset", err.Error())
	}
}

func (r *assetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, _, err := splitCompositeID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format organization_id/asset_id, got: "+req.ID)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyAssetAttrs copies API attributes into the model, preserving the
// create-only fields already set on the model where the API omits them.
func applyAssetAttrs(m *assetModel, attrs *client.AssetAttributes) {
	if attrs.AssetType != "" {
		m.AssetType = types.StringValue(attrs.AssetType)
	}
	if attrs.Identifier != "" {
		m.Identifier = types.StringValue(attrs.Identifier)
	}
	m.Description = optionalString(attrs.Description)
	m.MaxSeverity = types.StringValue(attrs.MaxSeverity)
	m.Reference = optionalString(attrs.Reference)
}

// optionalString maps an empty API value to null so an unset optional attribute
// does not show perpetual drift.
func optionalString(s string) types.String {
	if strings.TrimSpace(s) == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}
