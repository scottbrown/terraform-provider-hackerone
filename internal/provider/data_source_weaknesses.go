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
	_ datasource.DataSource              = &weaknessesDataSource{}
	_ datasource.DataSourceWithConfigure = &weaknessesDataSource{}
)

// NewWeaknessesDataSource lists a program's weakness (CWE/CAPEC) catalog. This
// list commonly spans many pages; the client pages through all of them.
func NewWeaknessesDataSource() datasource.DataSource { return &weaknessesDataSource{} }

type weaknessesDataSource struct {
	client *client.Client
}

type weaknessItem struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ExternalID  types.String `tfsdk:"external_id"`
	Description types.String `tfsdk:"description"`
}

type weaknessesDataSourceModel struct {
	ProgramID  types.String   `tfsdk:"program_id"`
	Weaknesses []weaknessItem `tfsdk:"weaknesses"`
}

func (d *weaknessesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_weaknesses"
}

func (d *weaknessesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the weakness (CWE/CAPEC) catalog available to a HackerOne program.",
		Attributes: map[string]schema.Attribute{
			"program_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Numeric ID of the program.",
			},
			"weaknesses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All weaknesses in the program's catalog, in API order (fully paginated).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "HackerOne weakness ID."},
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Weakness name."},
						"external_id": schema.StringAttribute{Computed: true, MarkdownDescription: "External identifier (e.g. `cwe-36`, `capec-597`)."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Weakness description."},
					},
				},
			},
		},
	}
}

func (d *weaknessesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData); ok {
		d.client = c
	}
}

func (d *weaknessesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg weaknessesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	items, err := d.client.ListWeaknesses(ctx, cfg.ProgramID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing weaknesses", err.Error())
		return
	}
	cfg.Weaknesses = make([]weaknessItem, 0, len(items))
	for _, it := range items {
		cfg.Weaknesses = append(cfg.Weaknesses, weaknessItem{
			ID:          types.StringValue(it.ID),
			Name:        types.StringValue(it.Name),
			ExternalID:  optionalString(it.ExternalID),
			Description: optionalString(it.Description),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
