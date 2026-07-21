// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

var (
	_ datasource.DataSource              = &programDataSource{}
	_ datasource.DataSourceWithConfigure = &programDataSource{}
)

// NewProgramDataSource resolves a program handle to its numeric ID (and other
// program metadata), which the write resources need as program_id.
func NewProgramDataSource() datasource.DataSource { return &programDataSource{} }

type programDataSource struct {
	client *client.Client
}

type programDataSourceModel struct {
	Handle         types.String `tfsdk:"handle"`
	ID             types.String `tfsdk:"id"`
	OrganizationID types.String `tfsdk:"organization_id"`
	Policy         types.String `tfsdk:"policy"`
}

func (d *programDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_program"
}

func (d *programDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a HackerOne program by handle, exposing its numeric ID for use with write resources.",
		Attributes: map[string]schema.Attribute{
			"handle": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The program handle (as shown in the program URL).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Numeric ID of the program.",
			},
			"organization_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Numeric ID of the organization that owns the program. Use this for the `organization_id` argument on `hackerone_asset` and `hackerone_asset_scope`.",
			},
			"policy": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Current program policy text.",
			},
		},
	}
}

func (d *programDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		d.client = c
	}
}

func (d *programDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg programDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, orgID, prog, err := d.client.GetProgramByHandle(ctx, cfg.Handle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up program", err.Error())
		return
	}
	cfg.ID = types.StringValue(id)
	cfg.OrganizationID = types.StringValue(orgID)
	cfg.Policy = types.StringValue(prog.Policy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
