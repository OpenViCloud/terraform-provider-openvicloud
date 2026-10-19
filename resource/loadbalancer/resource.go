package loadbalancer

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	timeout  = 10 * time.Minute
	interval = 5 * time.Second
	// The API takes 1 to 10 listeners and 1 to 20 backend VMs.
	maxListeners = 10
	maxBackends  = 20
)

var realClock = wait.Clock{Now: time.Now, Sleep: time.Sleep}

var listenerType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"protocol": types.StringType, "port": types.Int32Type, "target_port": types.Int32Type,
}}

type loadBalancer struct{ client *api.ClientWithResponses }

type model struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	NetworkId    types.String `tfsdk:"network_id"`
	Listeners    types.Set    `tfsdk:"listener"`
	BackendVmIds types.Set    `tfsdk:"backend_vm_ids"`
	Vip          types.String `tfsdk:"vip"`
	ZoneId       types.String `tfsdk:"zone_id"`
}

type listenerModel struct {
	Protocol   types.String `tfsdk:"protocol"`
	Port       types.Int32  `tfsdk:"port"`
	TargetPort types.Int32  `tfsdk:"target_port"`
}

func New() resource.Resource { return &loadBalancer{} }

var (
	_ resource.ResourceWithConfigure      = &loadBalancer{}
	_ resource.ResourceWithImportState    = &loadBalancer{}
	_ resource.ResourceWithValidateConfig = &loadBalancer{}
)

func (r *loadBalancer) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_load_balancer"
}

func (r *loadBalancer) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	port := []validator.Int32{int32validator.Between(1, 65535)}
	resp.Schema = schema.Schema{
		Description: "An L4 (TCP/UDP) load balancer on a network with public IPv6. Name, listeners and backend VMs " +
			"change in place and keep the VIP; another network replaces the load balancer (and gives it a new VIP). " +
			"OpenViCloud refuses to delete a VM a load balancer still sends to: remove it from backend_vm_ids before replacing it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Up to 100 letters, digits, hyphens and underscores, starting with a letter or digit; unique in the tenant.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`), "letters, digits, hyphens and underscores, starting with a letter or digit"),
				},
			},
			"network_id": schema.StringAttribute{
				Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description: "A network with enable_public_ipv6; the VIP comes from its /64.",
			},
			"listener": schema.SetNestedAttribute{
				Required: true, Description: "1 to 10 listeners; a protocol and port at most once.",
				Validators: []validator.Set{setvalidator.SizeBetween(1, maxListeners)},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"protocol":    schema.StringAttribute{Required: true, Validators: []validator.String{stringvalidator.OneOf("TCP", "UDP")}},
					"port":        schema.Int32Attribute{Required: true, Validators: port, Description: "Port on the VIP."},
					"target_port": schema.Int32Attribute{Required: true, Validators: port, Description: "Port on the backend VMs."},
				}},
			},
			"backend_vm_ids": schema.SetAttribute{
				ElementType: types.StringType, Required: true, Description: "1 to 20 VMs of the same network.",
				Validators: []validator.Set{setvalidator.SizeBetween(1, maxBackends)},
			},
			"vip":     schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "IPv6 address the load balancer listens on."},
			"zone_id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *loadBalancer) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Listeners.IsUnknown() || config.Listeners.IsNull() {
		return
	}
	var listeners []listenerModel
	resp.Diagnostics.Append(config.Listeners.ElementsAs(ctx, &listeners, false)...)
	seen := map[string]bool{}
	for _, l := range listeners {
		if l.Protocol.IsUnknown() || l.Port.IsUnknown() {
			continue
		}
		key := fmt.Sprintf("%s %d", l.Protocol.ValueString(), l.Port.ValueInt32())
		if seen[key] {
			resp.Diagnostics.AddAttributeError(path.Root("listener"), "Listener declared twice",
				fmt.Sprintf("%s port %d appears in two listeners; a protocol and port can listen once", l.Protocol.ValueString(), l.Port.ValueInt32()))
		}
		seen[key] = true
	}
}

func (r *loadBalancer) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *loadBalancer) get(ctx context.Context, id string) (int, *api.LoadBalancerQueryResult, error) {
	response, err := r.client.TitanInfrastructureEndpointsLoadBalancerGetLoadBalancerEndpointWithResponse(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode(), response.JSON200, nil
}

// waitFor polls the load balancer until check says done; a transport error or an answer without a body keeps waiting.
func (r *loadBalancer) waitFor(ctx context.Context, id string, check func(api.LoadBalancerQueryResult) (bool, error)) (api.LoadBalancerQueryResult, error) {
	var found api.LoadBalancerQueryResult
	err := wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		_, lb, err := r.get(ctx, id)
		if err != nil || lb == nil {
			return false, nil
		}
		found = *lb
		return check(found)
	})
	return found, err
}

func (r *loadBalancer) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	listeners, backends, diags := wanted(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := api.CreateLoadBalancerCommand{Name: plan.Name.ValueString(), NetworkId: plan.NetworkId.ValueString()}
	listenerInputs, backendInputs := inputs(listeners, backends)
	body.Listeners, body.Backends = &listenerInputs, &backendInputs

	id, err := api.SendCreate(time.Sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsLoadBalancerCreateLoadBalancerEndpointParams{IdempotencyKey: key}
		response, err := r.client.TitanInfrastructureEndpointsLoadBalancerCreateLoadBalancerEndpointWithResponse(ctx, params, body)
		if err != nil {
			return nil, nil, nil, err
		}
		var id *string
		if response.JSON202 != nil {
			id = response.JSON202.LoadBalancerId
		}
		return id, response.HTTPResponse, response.Body, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating the load balancer failed", err.Error())
		return
	}
	// The id goes into state before the wait: a create that fails later leaves a tainted resource that destroy can
	// clean up, not a load balancer Terraform does not know about.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)

	found, err := r.waitFor(ctx, id, Created)
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the load balancer failed", err.Error())
		return
	}
	resp.Diagnostics.Append(fill(ctx, &plan, found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *loadBalancer) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, found, err := r.get(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the load balancer failed", err.Error())
		return
	}
	if status == http.StatusNotFound || (found != nil && found.CurrentExistence != nil && *found.CurrentExistence == api.CurrentExistenceDeleted) {
		resp.State.RemoveResource(ctx)
		return
	}
	if found == nil {
		resp.Diagnostics.AddError("Reading the load balancer failed", fmt.Sprintf("the API answered %d", status))
		return
	}
	resp.Diagnostics.Append(fill(ctx, &state, *found)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *loadBalancer) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.Id.ValueString()

	if !plan.Name.Equal(state.Name) {
		response, err := r.client.TitanInfrastructureEndpointsLoadBalancerRenameLoadBalancerEndpointWithResponse(ctx, id,
			api.RenameLoadBalancerBody{Name: plan.Name.ValueString()})
		if err != nil {
			resp.Diagnostics.AddError("Renaming the load balancer failed", err.Error())
			return
		}
		if response.StatusCode() != http.StatusNoContent {
			resp.Diagnostics.AddError("Renaming the load balancer failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
			return
		}
		state.Name = plan.Name
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}

	if !plan.Listeners.Equal(state.Listeners) || !plan.BackendVmIds.Equal(state.BackendVmIds) {
		listeners, backends, diags := wanted(ctx, plan)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		// The API refuses a new configuration while another request is pending (a create or update that timed out
		// here): wait it out first.
		if _, err := r.waitFor(ctx, id, func(lb api.LoadBalancerQueryResult) (bool, error) { return Settled(lb), nil }); err != nil {
			resp.Diagnostics.AddError("Waiting for the load balancer's pending request failed", err.Error())
			return
		}
		listenerInputs, backendInputs := inputs(listeners, backends)
		response, err := r.client.TitanInfrastructureEndpointsLoadBalancerUpdateLoadBalancerEndpointWithResponse(ctx, id,
			api.UpdateLoadBalancerBody{Listeners: &listenerInputs, Backends: &backendInputs})
		if err != nil {
			resp.Diagnostics.AddError("Updating the load balancer failed", err.Error())
			return
		}
		if response.StatusCode() != http.StatusNoContent {
			resp.Diagnostics.AddError("Updating the load balancer failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
			return
		}
		found, err := r.waitFor(ctx, id, func(lb api.LoadBalancerQueryResult) (bool, error) { return Updated(lb, listeners, backends) })
		if err != nil {
			resp.Diagnostics.AddError("Waiting for the load balancer update failed", err.Error())
			return
		}
		resp.Diagnostics.Append(fill(ctx, &state, found)...)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *loadBalancer) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.Id.ValueString()
	// The API refuses a delete while the create is pending (a create that timed out here, an apply interrupted): wait
	// it out first, so destroy and replace work on a tainted load balancer.
	err := wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		_, lb, err := r.get(ctx, id)
		if err != nil {
			return false, nil
		}
		return Deletable(lb), nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the load balancer's pending request failed", err.Error())
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsLoadBalancerDeleteLoadBalancerEndpointWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Deleting the load balancer failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		return
	}
	if response.StatusCode() != http.StatusAccepted {
		resp.Diagnostics.AddError("Deleting the load balancer failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		status, lb, err := r.get(ctx, id)
		if err != nil {
			return false, nil
		}
		return Gone(status, lb)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the load balancer to be deleted failed", err.Error())
	}
}

func (r *loadBalancer) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// wanted reads the configured listeners and backend VMs out of the plan.
func wanted(ctx context.Context, m model) ([]Listener, []string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var listenerModels []listenerModel
	diags.Append(m.Listeners.ElementsAs(ctx, &listenerModels, false)...)
	var backends []string
	diags.Append(m.BackendVmIds.ElementsAs(ctx, &backends, false)...)
	var listeners []Listener
	for _, l := range listenerModels {
		listeners = append(listeners, Listener{Protocol: l.Protocol.ValueString(), Port: l.Port.ValueInt32(), TargetPort: l.TargetPort.ValueInt32()})
	}
	return listeners, backends, diags
}

func inputs(listeners []Listener, backends []string) ([]api.LoadBalancerListenerInput, []api.LoadBalancerBackendInput) {
	listenerInputs := []api.LoadBalancerListenerInput{}
	for _, l := range listeners {
		protocol, port, targetPort := l.Protocol, l.Port, l.TargetPort
		listenerInputs = append(listenerInputs, api.LoadBalancerListenerInput{Protocol: &protocol, Port: &port, TargetPort: &targetPort})
	}
	backendInputs := []api.LoadBalancerBackendInput{}
	for _, vmId := range backends {
		id := vmId
		backendInputs = append(backendInputs, api.LoadBalancerBackendInput{VmId: &id})
	}
	return listenerInputs, backendInputs
}

// fill copies what the API returns: the asked (desired) listeners and backends, not what Incus last took.
func fill(ctx context.Context, m *model, lb api.LoadBalancerQueryResult) diag.Diagnostics {
	var diags diag.Diagnostics
	m.Id = text(lb.Id)
	m.Name = text(lb.Name)
	m.NetworkId = text(lb.NetworkId)
	m.Vip = text(lb.Vip)
	m.ZoneId = text(lb.ZoneId)

	listeners := []attr.Value{}
	if lb.Listeners != nil {
		for _, l := range *lb.Listeners {
			object, d := types.ObjectValue(listenerType.AttrTypes, map[string]attr.Value{
				"protocol": types.StringValue(value(l.Protocol)), "port": types.Int32Value(number(l.Port)),
				"target_port": types.Int32Value(number(l.TargetPort)),
			})
			diags.Append(d...)
			listeners = append(listeners, object)
		}
	}
	set, d := types.SetValue(listenerType, listeners)
	diags.Append(d...)
	m.Listeners = set

	vms := []attr.Value{}
	if lb.Backends != nil {
		for _, b := range *lb.Backends {
			vms = append(vms, types.StringValue(value(b.VmId)))
		}
	}
	vmSet, d := types.SetValue(types.StringType, vms)
	diags.Append(d...)
	m.BackendVmIds = vmSet
	return diags
}

func text(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}
