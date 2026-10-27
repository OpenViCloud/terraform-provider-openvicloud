package datasource

import (
	"context"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type templates struct{ client *api.ClientWithResponses }

type templatesModel struct {
	ClusterId types.String    `tfsdk:"cluster_id"`
	Templates []templateModel `tfsdk:"templates"`
}

type templateModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Os          types.String `tfsdk:"os"`
	DiskGb      types.Int64  `tfsdk:"disk_gb"`
}

func NewTemplates() datasource.DataSource { return &templates{} }

func (d *templates) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_templates"
}

func (d *templates) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The operating systems a VM can start from. With cluster_id: only those ready in that cluster; without: those ready in at least one cluster.",
		Attributes: map[string]schema.Attribute{
			"cluster_id": schema.StringAttribute{Optional: true, Description: "Only the templates ready in this cluster."},
			"templates": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":          schema.StringAttribute{Computed: true},
					"name":        schema.StringAttribute{Computed: true},
					"description": schema.StringAttribute{Computed: true},
					"os":          schema.StringAttribute{Computed: true},
					"disk_gb":     schema.Int64Attribute{Computed: true},
				}},
			},
		},
	}
}

func (d *templates) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req, resp)
}

func (d *templates) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state templatesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	params := &api.TitanInfrastructureEndpointsCatalogListTemplatesEndpointParams{}
	if !state.ClusterId.IsNull() && state.ClusterId.ValueString() != "" {
		cluster := state.ClusterId.ValueString()
		params.ClusterId = &cluster
	}
	response, err := d.client.TitanInfrastructureEndpointsCatalogListTemplatesEndpointWithResponse(ctx, params)
	if err != nil {
		resp.Diagnostics.AddError("Listing templates failed", err.Error())
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Listing templates failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state.Templates = []templateModel{}
	for _, template := range *response.JSON200 {
		state.Templates = append(state.Templates, templateModel{
			Id: text(template.Id), Name: text(template.Name), Description: text(template.Description),
			Os: text(template.Os), DiskGb: whole(template.DiskGb),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
