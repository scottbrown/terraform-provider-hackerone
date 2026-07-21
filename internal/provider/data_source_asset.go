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
	_ datasource.DataSource              = &assetDataSource{}
	_ datasource.DataSourceWithConfigure = &assetDataSource{}
)

// NewAssetDataSource looks up an existing asset by its identifier, so config
// can reference an asset it does not manage (e.g. for asset_scope bindings)
// without hardcoding the numeric asset ID.
func NewAssetDataSource() datasource.DataSource { return &assetDataSource{} }

type assetDataSource struct {
	client *client.Client
}

type assetDataSourceModel struct {
	OrganizationID types.String `tfsdk:"organization_id"`
	Identifier     types.String `tfsdk:"identifier"`
	AssetID        types.String `tfsdk:"asset_id"`
	AssetType      types.String `tfsdk:"asset_type"`
	Description    types.String `tfsdk:"description"`
	MaxSeverity    types.String `tfsdk:"max_severity"`
	Reference      types.String `tfsdk:"reference"`
}

func (d *assetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_asset"
}

func (d *assetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an existing asset in an organization by its identifier, exposing its numeric `asset_id` for use with `hackerone_asset_scope`.",
		Attributes: map[string]schema.Attribute{
			"organization_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the organization that owns the asset.",
			},
			"identifier": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The asset identifier to look up (e.g. `example.com`). Must match an existing asset exactly.",
			},
			"asset_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The bare numeric asset ID.",
			},
			"asset_type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Asset type (e.g. `domain`, `url`, `wildcard`).",
			},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Free-text description of the asset.",
			},
			"max_severity": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Maximum severity for findings on this asset.",
			},
			"reference": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Internal reference identifier for the asset.",
			},
		},
	}
}

func (d *assetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		d.client = c
	}
}

func (d *assetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg assetDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, attrs, err := d.client.GetAssetByIdentifier(ctx, cfg.OrganizationID.ValueString(), cfg.Identifier.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up asset", err.Error())
		return
	}
	if id == "" {
		resp.Diagnostics.AddError(
			"Asset not found",
			"No asset with identifier "+cfg.Identifier.ValueString()+" was found in organization "+cfg.OrganizationID.ValueString()+".",
		)
		return
	}
	cfg.AssetID = types.StringValue(id)
	cfg.AssetType = types.StringValue(attrs.AssetType)
	cfg.Description = optionalString(attrs.Description)
	cfg.MaxSeverity = types.StringValue(attrs.MaxSeverity)
	cfg.Reference = optionalString(attrs.Reference)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
