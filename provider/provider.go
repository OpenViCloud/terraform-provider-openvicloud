// Package provider is the OpenViCloud provider itself: its configuration and the list of resources and data
// sources it offers.
package provider

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DefaultApiUrl is staging until OpenViCloud has a production host, the same default as the openvicloud CLI.
const DefaultApiUrl = "https://api-titan-staging.openvicloud.com"

// Settings picks the API address and token: the provider block first, then OPENVICLOUD_API_URL and
// OPENVICLOUD_TOKEN, then the staging address. problem is set when there is no token.
func Settings(configuredUrl string, configuredToken string, getenv func(string) string) (url string, token string, problem string) {
	url = strings.TrimSpace(configuredUrl)
	if url == "" {
		url = strings.TrimSpace(getenv("OPENVICLOUD_API_URL"))
	}
	if url == "" {
		url = DefaultApiUrl
	}
	token = strings.TrimSpace(configuredToken)
	if token == "" {
		token = strings.TrimSpace(getenv("OPENVICLOUD_TOKEN"))
	}
	if token == "" {
		problem = "no API token: set OPENVICLOUD_TOKEN, or token in the provider block, to a token created on the OpenViCloud portal (API tokens page)"
	}
	return url, token, problem
}

type openViCloud struct {
	version    string
	dataSource []func() datasource.DataSource
	resources  []func() resource.Resource
}

type configModel struct {
	ApiUrl types.String `tfsdk:"api_url"`
	Token  types.String `tfsdk:"token"`
}

// New returns the provider factory; dataSources and resources are the constructors it offers.
func New(version string, dataSources []func() datasource.DataSource, resources []func() resource.Resource) func() provider.Provider {
	return func() provider.Provider {
		return &openViCloud{version: version, dataSource: dataSources, resources: resources}
	}
}

func (p *openViCloud) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "openvicloud"
	resp.Version = p.version
}

func (p *openViCloud) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages resources of an OpenViCloud tenant through the public /v1 API, with an API token.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional:    true,
				Description: "Address of the OpenViCloud API. Defaults to OPENVICLOUD_API_URL, then " + DefaultApiUrl + ".",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token (ovc_…) created on the portal. Defaults to OPENVICLOUD_TOKEN.",
			},
		},
	}
}

func (p *openViCloud) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config configModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	url, token, problem := Settings(config.ApiUrl.ValueString(), config.Token.ValueString(), os.Getenv)
	if problem != "" {
		resp.Diagnostics.AddError("Missing API token", problem)
		return
	}
	client, err := api.New(url, token, p.version, &http.Client{Timeout: 60 * time.Second})
	if err != nil {
		resp.Diagnostics.AddError("Invalid api_url", err.Error())
		return
	}
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *openViCloud) DataSources(ctx context.Context) []func() datasource.DataSource {
	return p.dataSource
}

func (p *openViCloud) Resources(ctx context.Context) []func() resource.Resource {
	return p.resources
}
