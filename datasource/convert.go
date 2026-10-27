// Package datasource holds the read-only catalog data sources: clusters, templates and the two offering lists.
package datasource

import (
	"encoding/json"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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

func yesNo(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}

// exact keeps a decimal (a price, CPU units) as the digits the API sent: a float would round VND prices.
func exact(value *json.Number) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(value.String())
}

// clientOf reads the API client the provider's Configure handed over; nil before the provider is configured.
func clientOf(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *api.ClientWithResponses {
	if req.ProviderData == nil {
		return nil
	}
	client, ok := req.ProviderData.(*api.ClientWithResponses)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "the provider handed over something that is not an API client")
		return nil
	}
	return client
}
