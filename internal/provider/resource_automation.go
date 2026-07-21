// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

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
	_ resource.Resource                = &automationResource{}
	_ resource.ResourceWithConfigure   = &automationResource{}
	_ resource.ResourceWithImportState = &automationResource{}
)

// NewAutomationResource manages a program automation/trigger. Note that the
// API updates automations with PATCH (not PUT) and template_identifier is
// create-only.
func NewAutomationResource() resource.Resource { return &automationResource{} }

type automationResource struct {
	client *client.Client
}

type automationModel struct {
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Title          types.String `tfsdk:"title"`
	Code           types.String `tfsdk:"code"`
	Enabled        types.Bool   `tfsdk:"enabled"`
}

func (r *automationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_automation"
}

func (r *automationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a HackerOne automation (trigger) for an organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite ID in the form `organization_id/automation_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the organization.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"title": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable title of the automation.",
			},
			"code": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The automation code/script body.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the automation is active. Defaults to `true`.",
			},
		},
	}
}

func (r *automationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		r.client = c
	}
}

func (r *automationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan automationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID := plan.OrganizationID.ValueString()
	enabled := plan.Enabled.ValueBool()
	id, attrs, err := r.client.CreateAutomation(ctx, orgID, client.AutomationAttributes{
		Title:   plan.Title.ValueString(),
		Code:    plan.Code.ValueString(),
		Enabled: &enabled,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating automation", err.Error())
		return
	}
	plan.ID = types.StringValue(compositeID(orgID, id))
	applyAutomationAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *automationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state automationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, autoID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid automation ID", err.Error())
		return
	}
	attrs, err := r.client.GetAutomation(ctx, orgID, autoID)
	if err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading automation", err.Error())
		return
	}
	state.OrganizationID = types.StringValue(orgID)
	applyAutomationAttrs(&state, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *automationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state automationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, autoID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid automation ID", err.Error())
		return
	}
	enabled := plan.Enabled.ValueBool()
	attrs, err := r.client.UpdateAutomation(ctx, orgID, autoID, client.AutomationAttributes{
		Title:   plan.Title.ValueString(),
		Code:    plan.Code.ValueString(),
		Enabled: &enabled,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating automation", err.Error())
		return
	}
	plan.ID = state.ID
	applyAutomationAttrs(&plan, attrs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete: the public API exposes no DELETE for automations. Disable it to
// neutralize the trigger, then drop from state.
func (r *automationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state automationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID, autoID, err := splitCompositeID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid automation ID", err.Error())
		return
	}
	disabled := false
	if _, err := r.client.UpdateAutomation(ctx, orgID, autoID, client.AutomationAttributes{
		Title:   state.Title.ValueString(),
		Code:    state.Code.ValueString(),
		Enabled: &disabled,
	}); err != nil && !client.NotFound(err) {
		resp.Diagnostics.AddWarning(
			"Automation not deleted",
			"HackerOne's API has no endpoint to delete an automation; it was disabled instead and removed from Terraform state. Remove it manually in the console if needed. Error: "+err.Error(),
		)
	}
}

func (r *automationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, _, err := splitCompositeID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format organization_id/automation_id, got: "+req.ID)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func applyAutomationAttrs(m *automationModel, attrs *client.AutomationAttributes) {
	if attrs.Title != "" {
		m.Title = types.StringValue(attrs.Title)
	}
	if attrs.Code != "" {
		m.Code = types.StringValue(attrs.Code)
	}
	if attrs.Enabled != nil {
		m.Enabled = types.BoolValue(*attrs.Enabled)
	}
}
