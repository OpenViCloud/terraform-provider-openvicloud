package sshkey

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sshKey struct{ client *api.ClientWithResponses }

type model struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	PublicKey   types.String `tfsdk:"public_key"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	Type        types.String `tfsdk:"type"`
}

func New() resource.Resource { return &sshKey{} }

var (
	_ resource.ResourceWithConfigure   = &sshKey{}
	_ resource.ResourceWithImportState = &sshKey{}
)

func (r *sshKey) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

// replaceWhenAnotherKey replaces the key only when the configured key has another fingerprint than the stored one:
// after an import the state has no public_key (the API never returns it), and the same key must not be replaced.
func replaceWhenAnotherKey(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	var stored types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("fingerprint"), &stored)...)
	if resp.Diagnostics.HasError() || req.PlanValue.IsUnknown() {
		return
	}
	planned, err := Fingerprint(req.PlanValue.ValueString())
	resp.RequiresReplace = err != nil || planned != stored.ValueString()
}

type publicKeyValidator struct{}

func (publicKeyValidator) Description(ctx context.Context) string {
	return "an OpenSSH public key line"
}

func (v publicKeyValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (publicKeyValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := Fingerprint(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Not a public key", "public_key must be one OpenSSH public key line (ssh-ed25519 …, ssh-rsa …): "+err.Error())
	}
}

func (r *sshKey) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A public SSH key of the tenant; a VM created with it lets the key log in as user ubuntu. Keys cannot change: any change replaces the key.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"public_key": schema.StringAttribute{
				Required: true, Description: "One OpenSSH public key line.",
				Validators:    []validator.String{publicKeyValidator{}},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(replaceWhenAnotherKey, "replaced when the key has another fingerprint", "replaced when the key has another fingerprint")},
			},
			"fingerprint": schema.StringAttribute{Computed: true, PlanModifiers: keep, Description: "SHA256 fingerprint, as ssh-keygen -lf prints it."},
			"type":        schema.StringAttribute{Computed: true, PlanModifiers: keep},
		},
	}
}

func (r *sshKey) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sshKey) find(ctx context.Context, id string) (*api.SshKeyQueryResult, error) {
	response, err := r.client.TitanInfrastructureEndpointsSshKeyListSshKeysEndpointWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if response.JSON200 == nil {
		return nil, errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
	}
	for _, key := range *response.JSON200 {
		if key.Id != nil && *key.Id == id {
			found := key
			return &found, nil
		}
	}
	return nil, nil
}

func (r *sshKey) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	name, publicKey := plan.Name.ValueString(), plan.PublicKey.ValueString()
	id, err := api.SendCreate(time.Sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsSshKeyAddSshKeyEndpointParams{IdempotencyKey: key}
		response, err := r.client.TitanInfrastructureEndpointsSshKeyAddSshKeyEndpointWithResponse(ctx, params,
			api.AddSshKeyCommand{Name: &name, PublicKey: &publicKey})
		if err != nil {
			return nil, nil, nil, err
		}
		var id *string
		if response.JSON201 != nil {
			id = response.JSON201.Id
		}
		return id, response.HTTPResponse, response.Body, nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Adding the SSH key failed", err.Error())
		return
	}
	// The id goes into state before the read: a read that fails leaves a tainted resource that destroy can clean up,
	// not a key Terraform does not know about (adding it again would be refused as a duplicate).
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	found, err := r.find(ctx, id)
	if err != nil || found == nil {
		resp.Diagnostics.AddError("Reading the new SSH key failed", "the key was added but is not listed: "+errorText(err))
		return
	}
	fill(&plan, *found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKey) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.find(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading the SSH key failed", err.Error())
		return
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	fill(&state, *found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update only records the configured key: it is reached when an imported key gets its public_key from the
// configuration (same fingerprint); any other change replaces the key.
func (r *sshKey) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKey) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	response, err := r.client.TitanInfrastructureEndpointsSshKeyDeleteSshKeyEndpointWithResponse(ctx, state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Deleting the SSH key failed", err.Error())
		return
	}
	if response.StatusCode() != http.StatusNoContent && response.StatusCode() != http.StatusNotFound {
		resp.Diagnostics.AddError("Deleting the SSH key failed", api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
	}
}

func (r *sshKey) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// fill copies what the API returns; public_key stays as configured (the API never returns it).
func fill(m *model, key api.SshKeyQueryResult) {
	m.Id = text(key.Id)
	m.Name = text(key.Name)
	m.Fingerprint = text(key.Fingerprint)
	m.Type = text(key.Type)
}

func text(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func errorText(err error) string {
	if err == nil {
		return "not found in the list"
	}
	return err.Error()
}
