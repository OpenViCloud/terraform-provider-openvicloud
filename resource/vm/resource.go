package vm

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	timeout  = 10 * time.Minute
	interval = 5 * time.Second
	// The API refuses more than this many keys on one VM.
	maxSshKeys = 10
	// The API refuses user data longer than this, counted in UTF-8 bytes.
	maxUserDataBytes = 32768
)

var realClock = wait.Clock{Now: time.Now, Sleep: time.Sleep}

type vm struct{ client *api.ClientWithResponses }

type model struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	ZoneId            types.String `tfsdk:"zone_id"`
	NetworkId         types.String `tfsdk:"network_id"`
	TemplateId        types.String `tfsdk:"template_id"`
	VmOfferingId      types.String `tfsdk:"vm_offering_id"`
	DiskSizeGb        types.Int32  `tfsdk:"disk_size_gb"`
	SshKeyIds         types.Set    `tfsdk:"ssh_key_ids"`
	UserData          types.String `tfsdk:"user_data"`
	IpAddress         types.String `tfsdk:"ip_address"`
	PublicIpv6Address types.String `tfsdk:"public_ipv6_address"`
}

func New() resource.Resource { return &vm{} }

var (
	_ resource.ResourceWithConfigure   = &vm{}
	_ resource.ResourceWithImportState = &vm{}
)

func (r *vm) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

// replaceUnlessImported replaces the VM when its keys change, except after an import: the API never returns the keys,
// so an imported VM has none in state and the configured keys are only recorded.
func replaceUnlessImported(ctx context.Context, req planmodifier.SetRequest, resp *setplanmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.IsNull()
}

func (r *vm) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A virtual machine of the tenant. Only name changes in place; any other change replaces the VM. " +
			"Power is not managed here: stop and start a VM with the CLI or the portal.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Up to 64 letters, digits and hyphens, starting with a letter or digit; unique in the tenant.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(64),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`), "letters, digits and hyphens, starting with a letter or digit"),
				},
			},
			"zone_id":        schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Zone of the VM; the same as its network's."},
			"network_id":     schema.StringAttribute{Required: true, PlanModifiers: replace},
			"template_id":    schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Operating system (data source openvicloud_templates)."},
			"vm_offering_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Size of the VM (data source openvicloud_vm_offerings)."},
			"disk_size_gb": schema.Int32Attribute{
				Optional: true, Computed: true, Description: "Root disk size; the template's own size when not set (shown as 0).",
				PlanModifiers: []planmodifier.Int32{int32planmodifier.RequiresReplace(), int32planmodifier.UseStateForUnknown()},
			},
			"ssh_key_ids": schema.SetAttribute{
				ElementType: types.StringType, Optional: true, Computed: true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Description: "SSH keys (openvicloud_ssh_key) that may log in as user ubuntu.",
				Validators:  []validator.Set{setvalidator.SizeAtMost(maxSshKeys)},
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplaceIf(replaceUnlessImported,
					"replaced when the keys change", "replaced when the keys change")},
			},
			"user_data": schema.StringAttribute{
				Optional: true, Sensitive: true, PlanModifiers: replace,
				Description: "cloud-init user data, up to 32768 bytes.",
				Validators:  []validator.String{stringvalidator.LengthAtMost(maxUserDataBytes)},
			},
			"ip_address":          schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Private IPv4 address."},
			"public_ipv6_address": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "Public IPv6 address, when the network has a public /64."},
		},
	}
}

func (r *vm) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vm) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := api.CreateVmCommand{
		Name: plan.Name.ValueString(), ZoneId: plan.ZoneId.ValueString(), NetworkId: plan.NetworkId.ValueString(),
		TemplateId: plan.TemplateId.ValueString(), VmOfferingId: plan.VmOfferingId.ValueString(),
	}
	if !plan.DiskSizeGb.IsNull() && !plan.DiskSizeGb.IsUnknown() {
		size := plan.DiskSizeGb.ValueInt32()
		body.DiskSizeGb = &size
	}
	if !plan.UserData.IsNull() {
		userData := plan.UserData.ValueString()
		body.UserData = &userData
	}
	var keys []string
	resp.Diagnostics.Append(plan.SshKeyIds.ElementsAs(ctx, &keys, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(keys) > 0 {
		body.SshKeyIds = &keys
	}

	id, err := sendCreate(ctx, r.client, body, time.Sleep)
	if err != nil {
		resp.Diagnostics.AddError("Creating the VM failed", err.Error())
		return
	}
	// The id goes into state before the wait: a create that fails later leaves a tainted resource that destroy can
	// clean up, not a VM Terraform does not know about.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)

	var found api.VmQueryResult
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		response, err := r.client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, id)
		if err != nil {
			return false, nil
		}
		if response.JSON200 == nil {
			return false, errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		}
		found = *response.JSON200
		return Created(found)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the VM failed", err.Error())
		return
	}
	fill(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vm) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the VM failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Reading the VM failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	found := *response.JSON200
	if found.CurrentExistence != nil && *found.CurrentExistence == api.CurrentExistenceDeleted {
		resp.State.RemoveResource(ctx)
		return
	}
	fill(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vm) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Name is the only attribute that changes in place; ssh_key_ids reaches here only after an import, to be recorded.
	if !plan.Name.Equal(state.Name) {
		response, err := r.client.TitanInfrastructureEndpointsVmUpdateVmEndpointWithResponse(ctx, state.Id.ValueString(),
			api.RenameVmBody{Name: plan.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Renaming the VM failed", err.Error())
			return
		}
		if response.StatusCode() != http.StatusNoContent {
			resp.Diagnostics.AddError("Renaming the VM failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
			return
		}
	}
	state.Name = plan.Name
	state.SshKeyIds = plan.SshKeyIds
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vm) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.Id.ValueString()
	// The API refuses a delete while another request on the VM is pending (a create that timed out here, an apply
	// interrupted): wait it out first, so destroy and replace work on a tainted VM.
	err := wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		response, err := r.client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, id)
		if err != nil {
			return false, nil
		}
		return response.JSON200 == nil || Deletable(*response.JSON200), nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the VM's pending request failed", err.Error())
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsVmDeleteVmEndpointWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Deleting the VM failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		return
	}
	if response.StatusCode() != http.StatusAccepted {
		resp.Diagnostics.AddError("Deleting the VM failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		response, err := r.client.TitanInfrastructureEndpointsVmGetVmEndpointWithResponse(ctx, id)
		if err != nil {
			return false, nil
		}
		return Gone(response.StatusCode(), response.JSON200)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the VM to be deleted failed", err.Error())
	}
}

func (r *vm) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// fill copies what the API returns; ssh_key_ids stays as configured (the API never returns them).
func fill(m *model, v api.VmQueryResult) {
	m.Id = text(v.Id)
	m.Name = text(v.Name)
	m.ZoneId = text(v.ZoneId)
	m.NetworkId = text(v.NetworkId)
	m.TemplateId = text(v.TemplateId)
	m.VmOfferingId = text(v.VmOfferingId)
	if v.DiskSizeGb != nil {
		m.DiskSizeGb = types.Int32Value(*v.DiskSizeGb)
	}
	m.UserData = text(v.UserData)
	m.IpAddress = text(v.IpAddress)
	m.PublicIpv6Address = text(v.PublicIpv6Address)
}

func text(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}
