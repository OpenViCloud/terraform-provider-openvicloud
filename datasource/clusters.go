package datasource

import (
	"context"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clusters struct{ client *api.ClientWithResponses }

type clustersModel struct {
	Clusters []clusterModel `tfsdk:"clusters"`
}

type clusterModel struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewClusters() datasource.DataSource { return &clusters{} }

func (d *clusters) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clusters"
}

func (d *clusters) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The clusters (regions) a resource can be created in.",
		Attributes: map[string]schema.Attribute{
			"clusters": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{Computed: true, Description: "Cluster id, the cluster_id of other resources."},
					"name": schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *clusters) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req, resp)
}

func (d *clusters) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	response, err := d.client.TitanInfrastructureEndpointsCatalogListClustersEndpointWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing clusters failed", err.Error())
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Listing clusters failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state := clustersModel{Clusters: []clusterModel{}}
	for _, cluster := range *response.JSON200 {
		state.Clusters = append(state.Clusters, clusterModel{Id: text(cluster.Id), Name: text(cluster.Name)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
