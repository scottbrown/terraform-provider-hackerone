// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

var (
	_ resource.Resource                = &policyResource{}
	_ resource.ResourceWithConfigure   = &policyResource{}
	_ resource.ResourceWithImportState = &policyResource{}
)

// NewPolicyResource manages a program's policy prose (PUT /programs/{id}/policy).
// The policy is a singleton per program: create and update both PUT, and
// destroy is a no-op (there is no API to clear a policy, and an empty policy is
// not a meaningful remote state).
func NewPolicyResource() resource.Resource { return &policyResource{} }

type policyResource struct {
	client *client.Client
}

type policyModel struct {
	ProgramID types.String `tfsdk:"program_id"`
	Policy    types.String `tfsdk:"policy"`
}

func (r *policyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *policyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the security policy text for a HackerOne program.",
		Attributes: map[string]schema.Attribute{
			"program_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the program whose policy this manages. Use the `hackerone_program` data source to resolve a handle to an ID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"policy": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The program policy, in Markdown.",
			},
		},
	}
}

func (r *policyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		r.client = c
	}
}

func (r *policyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	attrs, err := r.client.UpdatePolicy(ctx, plan.ProgramID.ValueString(), plan.Policy.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting program policy", err.Error())
		return
	}
	plan.Policy = types.StringValue(attrs.Policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *policyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state policyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, prog, err := r.client.GetProgram(ctx, state.ProgramID.ValueString())
	if err != nil {
		if client.NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading program policy", err.Error())
		return
	}
	state.Policy = types.StringValue(prog.Policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *policyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	attrs, err := r.client.UpdatePolicy(ctx, plan.ProgramID.ValueString(), plan.Policy.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating program policy", err.Error())
		return
	}
	plan.Policy = types.StringValue(attrs.Policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is intentionally a no-op: HackerOne has no endpoint to remove a
// policy, and blanking it would leave a misleading empty program page. We drop
// the resource from state and leave the remote policy as-is.
func (r *policyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *policyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by program ID.
	resource.ImportStatePassthroughID(ctx, path.Root("program_id"), req, resp)
}
