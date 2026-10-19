package firewallrule

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// ruleBody is the create body written by hand: the spec declares the ports required numbers, but the API wants them
// absent for ICMP and All, which the generated CreateFirewallRuleRequest cannot express.
type ruleBody struct {
	Direction string `json:"direction"`
	Protocol  string `json:"protocol"`
	StartPort *int32 `json:"startPort,omitempty"`
	EndPort   *int32 `json:"endPort,omitempty"`
	Cidr      string `json:"cidr"`
}

func sendCreate(ctx context.Context, client *api.ClientWithResponses, networkId string, body ruleBody, sleep func(time.Duration)) (string, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return api.SendCreate(sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointParams{IdempotencyKey: key}
		response, err := client.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointWithBodyWithResponse(ctx, networkId, params,
			"application/json", bytes.NewReader(encoded))
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
