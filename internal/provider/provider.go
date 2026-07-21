// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/scottbrown/terraform-provider-hackerone/internal/client"
)

// Ensure the implementation satisfies the provider interface.
var _ provider.Provider = &hackeroneProvider{}

type hackeroneProvider struct {
	version string
}

// New returns a provider.Provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &hackeroneProvider{version: version}
	}
}

type providerModel struct {
	APIUsername types.String `tfsdk:"api_username"`
	APIToken    types.String `tfsdk:"api_token"`
	BaseURL     types.String `tfsdk:"base_url"`
}

func (p *hackeroneProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hackerone"
	resp.Version = p.version
}

func (p *hackeroneProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage HackerOne program configuration as code via the HackerOne v1 REST API.",
		Attributes: map[string]schema.Attribute{
			"api_username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HackerOne API identifier. Falls back to the `HACKERONE_API_IDENTIFIER` (or `HACKERONE_API_USERNAME`) environment variable.",
			},
			"api_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "HackerOne API token. Falls back to the `HACKERONE_API_TOKEN` environment variable. Prefer the env var so the secret stays out of state.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Override the API base URL. Defaults to `https://api.hackerone.com/v1`.",
			},
		},
	}
}

func (p *hackeroneProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// HACKERONE_API_IDENTIFIER matches HackerOne's own term for the value on the
	// API token settings page; HACKERONE_API_USERNAME is kept as a fallback.
	username := firstNonEmpty(
		cfg.APIUsername.ValueString(),
		os.Getenv("HACKERONE_API_IDENTIFIER"),
		os.Getenv("HACKERONE_API_USERNAME"),
	)
	token := firstNonEmpty(cfg.APIToken.ValueString(), os.Getenv("HACKERONE_API_TOKEN"))

	if username == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_username"),
			"Missing HackerOne API username",
			"Set the api_username argument or the HACKERONE_API_USERNAME environment variable.",
		)
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_token"),
			"Missing HackerOne API token",
			"Set the api_token argument or the HACKERONE_API_TOKEN environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	opts := []client.Option{client.WithUserAgent("terraform-provider-hackerone/" + p.version)}
	if base := cfg.BaseURL.ValueString(); base != "" {
		opts = append(opts, client.WithBaseURL(base))
	}

	c := client.New(username, token, opts...)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *hackeroneProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPolicyResource,
		NewScopeExclusionResource,
		NewAssetResource,
		NewAssetScopeResource,
		NewAutomationResource,
	}
}

func (p *hackeroneProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProgramDataSource,
		NewAssetDataSource,
		NewScopeExclusionsDataSource,
		NewWeaknessesDataSource,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
