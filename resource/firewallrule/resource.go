package firewallrule

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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

type firewallRule struct{ client *api.ClientWithResponses }

type model struct {
	Id        types.String `tfsdk:"id"`
	NetworkId types.String `tfsdk:"network_id"`
	Direction types.String `tfsdk:"direction"`
	Protocol  types.String `tfsdk:"protocol"`
	StartPort types.Int64  `tfsdk:"start_port"`
	EndPort   types.Int64  `tfsdk:"end_port"`
	Cidr      types.String `tfsdk:"cidr"`
}

func New() resource.Resource { return &firewallRule{} }

var (
	_ resource.ResourceWithConfigure      = &firewallRule{}
	_ resource.ResourceWithImportState    = &firewallRule{}
	_ resource.ResourceWithValidateConfig = &firewallRule{}
)

func (r *firewallRule) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *firewallRule) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	port := func(description string) schema.Int64Attribute {
		return schema.Int64Attribute{
			Optional: true, Computed: true, Description: description,
			PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace(), int64planmodifier.UseStateForUnknown()},
			Validators:    []validator.Int64{int64validator.Between(1, 65535)},
		}
	}
	resp.Schema = schema.Schema{
		Description: "One rule in the firewall (ACL) of a network. Every change replaces the rule.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"network_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Network whose firewall holds the rule."},
			"direction": schema.StringAttribute{
				Required: true, PlanModifiers: replace, Validators: []validator.String{stringvalidator.OneOf("Ingress", "Egress")},
			},
			"protocol": schema.StringAttribute{
				Required: true, PlanModifiers: replace, Validators: []validator.String{stringvalidator.OneOf("TCP", "UDP", "ICMP", "All")},
			},
			"start_port": port("First port, required for TCP and UDP."),
			"end_port":   port("Last port, required for TCP and UDP; at least start_port."),
			"cidr":       schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Addresses the rule applies to, e.g. 0.0.0.0/0."},
		},
	}
}

// ValidateConfig refuses at plan time what the API refuses at create time: TCP and UDP need both ports, in order;
// ICMP and All take none.
func (r *firewallRule) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Protocol.IsUnknown() {
		return
	}
	protocol := config.Protocol.ValueString()
	if protocol != "TCP" && protocol != "UDP" {
		if !config.StartPort.IsNull() || !config.EndPort.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("start_port"), "No ports for "+protocol, "ICMP and All rules take no start_port or end_port.")
		}
		return
	}
	if config.StartPort.IsNull() || config.EndPort.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("start_port"), "Ports required", "TCP and UDP rules need start_port and end_port.")
		return
	}
	if !config.StartPort.IsUnknown() && !config.EndPort.IsUnknown() && config.StartPort.ValueInt64() > config.EndPort.ValueInt64() {
		resp.Diagnostics.AddAttributeError(path.Root("end_port"), "Ports out of order", "end_port must be at least start_port.")
	}
}

func (r *firewallRule) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// list returns the rules of a network; found is false when the network itself is gone.
func (r *firewallRule) list(ctx context.Context, networkId string) (rules []api.FirewallRuleQueryResult, found bool, err error) {
	response, err := r.client.TitanInfrastructureEndpointsNetworkListFirewallRulesEndpointWithResponse(ctx, networkId)
	if err != nil {
		return nil, false, err
	}
	if response.StatusCode() == http.StatusNotFound {
		return nil, false, nil
	}
	if response.JSON200 == nil {
		return nil, false, errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
	}
	return *response.JSON200, true, nil
}

func (r *firewallRule) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := ruleBody{Direction: plan.Direction.ValueString(), Protocol: plan.Protocol.ValueString(), Cidr: plan.Cidr.ValueString()}
	if !plan.StartPort.IsNull() && !plan.StartPort.IsUnknown() {
		start := int32(plan.StartPort.ValueInt64())
		body.StartPort = &start
	}
	if !plan.EndPort.IsNull() && !plan.EndPort.IsUnknown() {
		end := int32(plan.EndPort.ValueInt64())
		body.EndPort = &end
	}
	networkId := plan.NetworkId.ValueString()
	id, err := sendCreate(ctx, r.client, networkId, body, time.Sleep)
	if err != nil {
		resp.Diagnostics.AddError("Creating the firewall rule failed", err.Error())
		return
	}
	// The ids go into state before the wait: a create that fails later leaves a tainted resource destroy can clean up.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_id"), networkId)...)

	var created api.FirewallRuleQueryResult
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		rules, found, err := r.list(ctx, networkId)
		if err != nil || !found {
			return false, err
		}
		rule := Find(rules, id)
		if rule == nil {
			return false, nil
		}
		created = *rule
		return Created(created)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the firewall rule failed", err.Error())
		return
	}
	fill(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallRule) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, found, err := r.list(ctx, state.NetworkId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the firewall rule failed", err.Error())
		return
	}
	rule := Find(rules, state.Id.ValueString())
	if !found || rule == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	fill(&state, *rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: every attribute requires replacement.
func (r *firewallRule) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Firewall rules do not change in place", "every attribute replaces the rule; this is a provider bug")
}

func (r *firewallRule) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	networkId, id := state.NetworkId.ValueString(), state.Id.ValueString()
	response, err := r.client.TitanInfrastructureEndpointsNetworkDeleteFirewallRuleEndpointWithResponse(ctx, networkId, id)
	if err != nil {
		resp.Diagnostics.AddError("Deleting the firewall rule failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		return
	}
	if response.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Deleting the firewall rule failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		rules, found, err := r.list(ctx, networkId)
		if err != nil {
			return false, err
		}
		if !found {
			return true, nil
		}
		return Gone(Find(rules, id))
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the firewall rule to be deleted failed", err.Error())
	}
}

func (r *firewallRule) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	networkId, id, ok := SplitImportId(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid import id", fmt.Sprintf("expected <network id>/<rule id>, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network_id"), networkId)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func fill(m *model, rule api.FirewallRuleQueryResult) {
	m.Id = text(rule.Id)
	m.Direction = text(rule.Direction)
	m.Protocol = text(rule.Protocol)
	m.Cidr = text(rule.Cidr)
	m.StartPort = whole(rule.StartPort)
	m.EndPort = whole(rule.EndPort)
}

func text(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func whole(value *int32) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*value))
}
