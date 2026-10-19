package volume

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	timeout  = 10 * time.Minute
	interval = 5 * time.Second
)

var realClock = wait.Clock{Now: time.Now, Sleep: time.Sleep}

type volume struct{ client *api.ClientWithResponses }

type model struct {
	Id               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ZoneId           types.String `tfsdk:"zone_id"`
	VolumeOfferingId types.String `tfsdk:"volume_offering_id"`
	SizeGb           types.Int32  `tfsdk:"size_gb"`
}

func New() resource.Resource { return &volume{} }

var (
	_ resource.ResourceWithConfigure   = &volume{}
	_ resource.ResourceWithImportState = &volume{}
)

func (r *volume) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume"
}

func (r *volume) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A block volume of the tenant; attach it to a VM with openvicloud_volume_attachment. Any change replaces the volume.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{
				Required: true, PlanModifiers: replace, Validators: []validator.String{stringvalidator.LengthBetween(1, 100)},
			},
			"zone_id":            schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Zone of the volume; the same as the VM it attaches to."},
			"volume_offering_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Kind of volume (data source openvicloud_volume_offerings)."},
			"size_gb": schema.Int32Attribute{
				Required: true, PlanModifiers: []planmodifier.Int32{int32planmodifier.RequiresReplace()},
				Validators: []validator.Int32{int32validator.AtLeast(1)},
			},
		},
	}
}

func (r *volume) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*api.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "the provider handed over something that is not an API client")
		return
	}
	r.client = client
}

// Lookup is the volume with this id from the list, or nil when it is not listed.
func Lookup(ctx context.Context, client *api.ClientWithResponses, id string) (*api.VolumeQueryResult, error) {
	response, err := client.TitanInfrastructureEndpointsVolumeListVolumesEndpointWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
	}
	return Find(*response.JSON200, id), nil
}

func (r *volume) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	size := plan.SizeGb.ValueInt32()
	body := api.CreateVolumeCommand{
		Name: plan.Name.ValueString(), ZoneId: plan.ZoneId.ValueString(),
		VolumeOfferingId: plan.VolumeOfferingId.ValueString(), SizeGb: &size,
	}
	id, err := api.SendCreate(time.Sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsVolumeCreateVolumeEndpointParams{IdempotencyKey: key}
		response, err := r.client.TitanInfrastructureEndpointsVolumeCreateVolumeEndpointWithResponse(ctx, params, body)
		if err != nil {
			return nil, nil, nil, err
		}
		var id *string
		if response.JSON202 != nil {
			id = response.JSON202.VolumeId
		}
		return id, response.HTTPResponse, response.Body, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating the volume failed", err.Error())
		return
	}
	// The id goes into state before the wait: a create that fails later leaves a tainted resource that destroy can
	// clean up, not a volume Terraform does not know about.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)

	var found *api.VolumeQueryResult
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		listed, err := Lookup(ctx, r.client, id)
		if err != nil {
			return false, nil
		}
		found = listed
		return Created(found)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume failed", err.Error())
		return
	}
	fill(&plan, *found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *volume) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := Lookup(ctx, r.client, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the volume failed", err.Error())
		return
	}
	if found == nil || (found.CurrentExistence != nil && *found.CurrentExistence == api.CurrentExistenceDeleted) {
		resp.State.RemoveResource(ctx)
		return
	}
	fill(&state, *found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached: every attribute replaces the volume.
func (r *volume) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("A volume cannot change", "every attribute of openvicloud_volume replaces it")
}

func (r *volume) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.Id.ValueString()
	// The API refuses a delete while another request on the volume is pending (a create that timed out here, an apply
	// interrupted): wait it out first, so destroy and replace work on a tainted volume.
	err := wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		listed, err := Lookup(ctx, r.client, id)
		if err != nil {
			return false, nil
		}
		return Deletable(listed), nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume's pending request failed", err.Error())
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsVolumeDeleteVolumeEndpointWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Deleting the volume failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		return
	}
	if response.StatusCode() != http.StatusAccepted {
		resp.Diagnostics.AddError("Deleting the volume failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		listed, err := Lookup(ctx, r.client, id)
		if err != nil {
			return false, nil
		}
		return Gone(listed)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume to be deleted failed", err.Error())
	}
}

func (r *volume) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func fill(m *model, v api.VolumeQueryResult) {
	m.Id = text(v.Id)
	m.Name = text(v.Name)
	m.ZoneId = text(v.ZoneId)
	m.VolumeOfferingId = text(v.VolumeOfferingId)
	if v.SizeGb != nil {
		m.SizeGb = types.Int32Value(*v.SizeGb)
	}
}

func text(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}
