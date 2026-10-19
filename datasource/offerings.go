package datasource

import (
	"context"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Prices and CPU units are strings: they keep every digit the API sent (VND, per hour).

type vmOfferings struct{ client *api.ClientWithResponses }

type vmOfferingsModel struct {
	Offerings []vmOfferingModel `tfsdk:"vm_offerings"`
}

type vmOfferingModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Cpu          types.String `tfsdk:"cpu"`
	Vcpu         types.Int64  `tfsdk:"vcpu"`
	MemoryMb     types.Int64  `tfsdk:"memory_mb"`
	PricePerHour types.String `tfsdk:"price_per_hour"`
}

func NewVmOfferings() datasource.DataSource { return &vmOfferings{} }

func (d *vmOfferings) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_offerings"
}

func (d *vmOfferings) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The VM sizes and their price per hour in VND.",
		Attributes: map[string]schema.Attribute{
			"vm_offerings": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":             schema.StringAttribute{Computed: true},
					"name":           schema.StringAttribute{Computed: true},
					"cpu":            schema.StringAttribute{Computed: true, Description: "CPU units, exact decimal."},
					"vcpu":           schema.Int64Attribute{Computed: true},
					"memory_mb":      schema.Int64Attribute{Computed: true},
					"price_per_hour": schema.StringAttribute{Computed: true, Description: "VND per hour, exact decimal."},
				}},
			},
		},
	}
}

func (d *vmOfferings) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req, resp)
}

func (d *vmOfferings) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	response, err := d.client.TitanInfrastructureEndpointsCatalogListVmOfferingsEndpointWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing VM offerings failed", err.Error())
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Listing VM offerings failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state := vmOfferingsModel{Offerings: []vmOfferingModel{}}
	for _, offering := range *response.JSON200 {
		state.Offerings = append(state.Offerings, vmOfferingModel{
			Id: text(offering.Id), Name: text(offering.Name), Cpu: exact(offering.Cpu), Vcpu: whole(offering.Vcpu),
			MemoryMb: whole(offering.MemoryMb), PricePerHour: exact(offering.PricePerHour),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

type volumeOfferings struct{ client *api.ClientWithResponses }

type volumeOfferingsModel struct {
	Offerings []volumeOfferingModel `tfsdk:"volume_offerings"`
}

type volumeOfferingModel struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	SizeGb            types.Int64  `tfsdk:"size_gb"`
	IsCustomized      types.Bool   `tfsdk:"is_customized"`
	PricePerGbPerHour types.String `tfsdk:"price_per_gb_per_hour"`
}

func NewVolumeOfferings() datasource.DataSource { return &volumeOfferings{} }

func (d *volumeOfferings) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_offerings"
}

func (d *volumeOfferings) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The volume sizes and their price per GB per hour in VND.",
		Attributes: map[string]schema.Attribute{
			"volume_offerings": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                    schema.StringAttribute{Computed: true},
					"name":                  schema.StringAttribute{Computed: true},
					"size_gb":               schema.Int64Attribute{Computed: true},
					"is_customized":         schema.BoolAttribute{Computed: true},
					"price_per_gb_per_hour": schema.StringAttribute{Computed: true, Description: "VND per GB per hour, exact decimal."},
				}},
			},
		},
	}
}

func (d *volumeOfferings) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientOf(req, resp)
}

func (d *volumeOfferings) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	response, err := d.client.TitanInfrastructureEndpointsCatalogListVolumeOfferingsEndpointWithResponse(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Listing volume offerings failed", err.Error())
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Listing volume offerings failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state := volumeOfferingsModel{Offerings: []volumeOfferingModel{}}
	for _, offering := range *response.JSON200 {
		state.Offerings = append(state.Offerings, volumeOfferingModel{
			Id: text(offering.Id), Name: text(offering.Name), SizeGb: whole(offering.SizeGb),
			IsCustomized: yesNo(offering.IsCustomized), PricePerGbPerHour: exact(offering.PricePerGbPerHour),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
