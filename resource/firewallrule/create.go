package firewallrule

import (
	"context"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func sendCreate(ctx context.Context, client *api.ClientWithResponses, networkId string, body api.CreateFirewallRuleRequest, sleep func(time.Duration)) (string, error) {
	return api.SendCreate(sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointParams{IdempotencyKey: key}
		response, err := client.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointWithResponse(ctx, networkId, params, body)
		if err != nil {
			return nil, nil, nil, err
		}
		var id *string
		if response.JSON201 != nil {
			id = response.JSON201.RuleId
		}
		return id, response.HTTPResponse, response.Body, nil
	})
}
