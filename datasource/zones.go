package datasource

import (
	"context"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type zones struct{ client *api.ClientWithResponses }

type zonesModel struct {
	Zones []zoneModel `tfsdk:"zones"`
}

type zoneModel struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

func NewZones() datasource.DataSource { return &zones{} }

func (d *zones) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_zones"
}

func (d *zones) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The zones (regions) a resource can be created in.",
		Attributes: map[string]schema.Attribute{
			"zones": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{Computed: true, Description: "Zone id, the zone_id of other resources."},
					"name": schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (d *zones) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req, resp)
}

func (d *zones) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	response, err := d.client.TitanInfrastructureEndpointsCatalogListZonesEndpointWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing zones failed", err.Error())
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Listing zones failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state := zonesModel{Zones: []zoneModel{}}
	for _, zone := range *response.JSON200 {
		state.Zones = append(state.Zones, zoneModel{Id: text(zone.Id), Name: text(zone.Name)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
