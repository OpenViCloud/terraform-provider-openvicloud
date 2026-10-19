package network

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/OpenViCloud/terraform-provider-openvicloud/wait"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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

// The API takes the policy as a number and returns it as a word (FirewallPolicy has no string converter); the
// configuration uses the word, so Read compares like with like.
var policyNumbers = map[string]api.FirewallPolicy{"Allow": 0, "Deny": 1}

type network struct{ client *api.ClientWithResponses }

type model struct {
	Id                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	ZoneId               types.String `tfsdk:"zone_id"`
	Cidr                 types.String `tfsdk:"cidr"`
	Gateway              types.String `tfsdk:"gateway"`
	DefaultIngressPolicy types.String `tfsdk:"default_ingress_policy"`
	DefaultEgressPolicy  types.String `tfsdk:"default_egress_policy"`
	EnablePublicIpv6     types.Bool   `tfsdk:"enable_public_ipv6"`
	PublicIpv6Subnet     types.String `tfsdk:"public_ipv6_subnet"`
}

func New() resource.Resource { return &network{} }

var (
	_ resource.ResourceWithConfigure   = &network{}
	_ resource.ResourceWithImportState = &network{}
)

func (r *network) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *network) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	replaceOrKeep := []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}
	policy := []validator.String{stringvalidator.OneOf("Allow", "Deny")}
	resp.Schema = schema.Schema{
		Description: "A private network of the tenant. Only name changes in place; any other change replaces the network.",
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
			"zone_id": schema.StringAttribute{
				Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description: "Zone of the network (data source openvicloud_zones).",
			},
			"cidr": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: replaceOrKeep,
				Description: "IPv4 range of the network; chosen by OpenViCloud when not set.",
			},
			"gateway": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"default_ingress_policy": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: replaceOrKeep, Validators: policy,
				Description: "Allow or Deny: what happens to inbound traffic no firewall rule matches.",
			},
			"default_egress_policy": schema.StringAttribute{
				Optional: true, Computed: true, PlanModifiers: replaceOrKeep, Validators: policy,
				Description: "Allow or Deny: what happens to outbound traffic no firewall rule matches.",
			},
			"enable_public_ipv6": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
				Description:   "Give the network a public IPv6 /64 of its zone.",
			},
			"public_ipv6_subnet": schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *network) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *network) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := api.CreateNetworkCommand{Name: plan.Name.ValueString(), ZoneId: plan.ZoneId.ValueString()}
	if !plan.Cidr.IsNull() && !plan.Cidr.IsUnknown() {
		cidr := plan.Cidr.ValueString()
		body.Cidr = &cidr
	}
	if !plan.DefaultIngressPolicy.IsNull() && !plan.DefaultIngressPolicy.IsUnknown() {
		policy := policyNumbers[plan.DefaultIngressPolicy.ValueString()]
		body.DefaultIngressPolicy = &policy
	}
	if !plan.DefaultEgressPolicy.IsNull() && !plan.DefaultEgressPolicy.IsUnknown() {
		policy := policyNumbers[plan.DefaultEgressPolicy.ValueString()]
		body.DefaultEgressPolicy = &policy
	}
	ipv6 := plan.EnablePublicIpv6.ValueBool()
	body.EnablePublicIpv6 = &ipv6

	id, err := sendCreate(ctx, r.client, body, time.Sleep)
	if err != nil {
		resp.Diagnostics.AddError("Creating the network failed", err.Error())
		return
	}
	// The id goes into state before the wait: a create that fails later leaves a tainted resource that destroy can
	// clean up, not a network Terraform does not know about.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)

	var found api.NetworkQueryResult
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		response, err := r.client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, id)
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
		resp.Diagnostics.AddError("Waiting for the network failed", err.Error())
		return
	}
	fill(&plan, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *network) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the network failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if response.JSON200 == nil {
		resp.Diagnostics.AddError("Reading the network failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
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

func (r *network) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Name is the only attribute without RequiresReplace.
	response, err := r.client.TitanInfrastructureEndpointsNetworkRenameNetworkEndpointWithResponse(ctx, state.Id.ValueString(),
		api.RenameNetworkRequest{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Renaming the network failed", err.Error())
		return
	}
	if response.StatusCode() != http.StatusNoContent {
		resp.Diagnostics.AddError("Renaming the network failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	state.Name = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *network) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.Id.ValueString()
	response, err := r.client.TitanInfrastructureEndpointsNetworkDeleteNetworkEndpointWithResponse(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Deleting the network failed", err.Error())
		return
	}
	if response.StatusCode() == http.StatusNotFound {
		return
	}
	if response.StatusCode() != http.StatusAccepted {
		resp.Diagnostics.AddError("Deleting the network failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		return
	}
	err = wait.Until(ctx, realClock, timeout, interval, func(ctx context.Context) (bool, error) {
		response, err := r.client.TitanInfrastructureEndpointsNetworkGetNetworkEndpointWithResponse(ctx, id)
		if err != nil {
			return false, nil
		}
		return Gone(response.StatusCode(), response.JSON200)
	})
	if err != nil {
		resp.Diagnostics.AddError("Waiting for the network to be deleted failed", err.Error())
	}
}

func (r *network) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func fill(m *model, n api.NetworkQueryResult) {
	m.Id = text(n.Id)
	m.Name = text(n.Name)
	m.ZoneId = text(n.ZoneId)
	m.Cidr = text(n.Cidr)
	m.Gateway = text(n.Gateway)
	m.DefaultIngressPolicy = text(n.DefaultIngressPolicy)
	m.DefaultEgressPolicy = text(n.DefaultEgressPolicy)
	m.PublicIpv6Subnet = text(n.PublicIpv6Subnet)
	m.EnablePublicIpv6 = types.BoolValue(n.PublicIpv6Subnet != nil)
}

func text(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}
