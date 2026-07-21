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
	_ datasource.DataSource              = &scopeExclusionsDataSource{}
	_ datasource.DataSourceWithConfigure = &scopeExclusionsDataSource{}
)

// NewScopeExclusionsDataSource lists all out-of-scope entries for a program.
func NewScopeExclusionsDataSource() datasource.DataSource { return &scopeExclusionsDataSource{} }

type scopeExclusionsDataSource struct {
	client *client.Client
}

type scopeExclusionItem struct {
	ID       types.String `tfsdk:"id"`
	Category types.String `tfsdk:"category"`
	Details  types.String `tfsdk:"details"`
}

type scopeExclusionsDataSourceModel struct {
	ProgramID  types.String         `tfsdk:"program_id"`
	Exclusions []scopeExclusionItem `tfsdk:"exclusions"`
}

func (d *scopeExclusionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scope_exclusions"
}

func (d *scopeExclusionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all scope exclusions (out-of-scope entries) for a HackerOne program.",
		Attributes: map[string]schema.Attribute{
			"program_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the program.",
			},
			"exclusions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All scope exclusions for the program, in API order.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":       schema.StringAttribute{Computed: true, MarkdownDescription: "Scope exclusion ID."},
						"category": schema.StringAttribute{Computed: true, MarkdownDescription: "Exclusion category."},
						"details":  schema.StringAttribute{Computed: true, MarkdownDescription: "Exclusion details."},
					},
				},
			},
		},
	}
}

func (d *scopeExclusionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		d.client = c
	}
}

func (d *scopeExclusionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg scopeExclusionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.ListScopeExclusions(ctx, cfg.ProgramID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing scope exclusions", err.Error())
		return
	}
	cfg.Exclusions = make([]scopeExclusionItem, 0, len(items))
	for _, it := range items {
		cfg.Exclusions = append(cfg.Exclusions, scopeExclusionItem{
			ID:       types.StringValue(it.ID),
			Category: types.StringValue(it.Category),
			Details:  optionalString(it.Details),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
