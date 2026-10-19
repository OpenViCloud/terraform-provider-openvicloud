// Package volumeattachment is the openvicloud_volume_attachment resource: a volume attached to a VM. A volume has at
// most one VM, so the attachment's id is the volume's id. It reads the volume through package volume (the API has
// no attachment of its own).
package volumeattachment

import (
	"context"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/resource/volume"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	timeout  = 10 * time.Minute
	interval = 5 * time.Second
)

var realClock = wait.Clock{Now: time.Now, Sleep: time.Sleep}

type attachment struct{ client *api.ClientWithResponses }

type model struct {
	Id       types.String `tfsdk:"id"`
	VolumeId types.String `tfsdk:"volume_id"`
	VmId     types.String `tfsdk:"vm_id"`
}

func New() resource.Resource { return &attachment{} }

var (
	_ resource.ResourceWithConfigure   = &attachment{}
	_ resource.ResourceWithImportState = &attachment{}
)

func (r *attachment) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume_attachment"
}

func (r *attachment) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Attaches a volume to a VM of the same zone. Changing either side detaches and attaches again.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, Description: "The volume's id."},
			"volume_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"vm_id":     schema.StringAttribute{Required: true, PlanModifiers: replace},
		},
	}
}

func (r *attachment) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *attachment) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	volumeId, vmId := plan.VolumeId.ValueString(), plan.VmId.ValueString()
	_, err := api.SendRepeatable(time.Sleep, func() (*http.Response, []byte, error) {
		response, err := r.client.TitanInfrastructureEndpointsVolumeAttachVolumeEndpointWithResponse(ctx, volumeId, api.AttachVolumeBody{VmId: vmId})
		if err != nil {
			return nil, nil, err
		}
		return response.HTTPResponse, response.Body, nil
	}, http.StatusAccepted)
	if err != nil {
		resp.Diagnostics.AddError("Attaching the volume failed", err.Error())
		return
	}
	// The id goes into state before the wait: an attach that fails later leaves a tainted resource that destroy
	// detaches.
	plan.Id = plan.VolumeId
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), volumeId)...)

	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		found, err := volume.Lookup(ctx, r.client, volumeId)
		if err != nil {
			return false, nil
		}
		return volume.Attached(found, vmId)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume to attach failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps the attachment only while Incus shows the volume on this VM. Anything else (detached, on another VM or
// an instance outside Titan, volume gone) removes it from state, so plan proposes to attach again rather than
// failing every plan and destroy.
func (r *attachment) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := volume.Lookup(ctx, r.client, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the volume failed", err.Error())
		return
	}
	if found == nil || found.CurrentAttachedVmId == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	imported := state.VmId.IsNull()
	if !imported && *found.CurrentAttachedVmId != state.VmId.ValueString() {
		resp.State.RemoveResource(ctx)
		return
	}
	state.VolumeId = state.Id
	state.VmId = types.StringValue(*found.CurrentAttachedVmId)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never reached: both attributes replace the attachment.
func (r *attachment) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("An attachment cannot change", "both attributes of openvicloud_volume_attachment replace it")
}

func (r *attachment) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	volumeId := state.Id.ValueString()
	// The API refuses a detach while another request on the volume is pending (an attach that timed out here, an
	// apply interrupted): wait it out first, so destroy and replace work on a tainted attachment.
	err := wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		found, err := volume.Lookup(ctx, r.client, volumeId)
		if err != nil {
			return false, nil
		}
		return volume.Detachable(found), nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume's pending request failed", err.Error())
		return
	}
	status, err := api.SendRepeatable(time.Sleep, func() (*http.Response, []byte, error) {
		response, err := r.client.TitanInfrastructureEndpointsVolumeDetachVolumeEndpointWithResponse(ctx, volumeId)
		if err != nil {
			return nil, nil, err
		}
		return response.HTTPResponse, response.Body, nil
	}, http.StatusAccepted, http.StatusNotFound)
	if err != nil {
		resp.Diagnostics.AddError("Detaching the volume failed", err.Error())
		return
	}
	if status == http.StatusNotFound {
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		found, err := volume.Lookup(ctx, r.client, volumeId)
		if err != nil {
			return false, nil
		}
		return volume.Detached(found)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the volume to detach failed", err.Error())
	}
}

// ImportState takes the volume's id; Read fills vm_id from the VM the volume is attached to.
func (r *attachment) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
