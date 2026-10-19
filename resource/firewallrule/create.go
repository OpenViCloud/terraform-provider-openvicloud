package firewallrule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

const createAttempts = 3

// ruleBody is the create body written by hand: the spec declares the ports required numbers, but the API wants them
// absent for ICMP and All, which the generated CreateFirewallRuleRequest cannot express.
type ruleBody struct {
	Direction string `json:"direction"`
	Protocol  string `json:"protocol"`
	StartPort *int32 `json:"startPort,omitempty"`
	EndPort   *int32 `json:"endPort,omitempty"`
	Cidr      string `json:"cidr"`
}

// sendCreate sends the create, again with the same Idempotency-Key while its answer is lost, and returns the rule id.
func sendCreate(ctx context.Context, client *api.ClientWithResponses, networkId string, body ruleBody, sleep func(time.Duration)) (string, error) {
	params := &api.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointParams{IdempotencyKey: api.NewIdempotencyKey()}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 1; attempt <= createAttempts; attempt++ {
		if attempt > 1 {
			sleep(2 * time.Second)
		}
		response, err := client.TitanInfrastructureEndpointsNetworkCreateFirewallRuleEndpointWithBodyWithResponse(ctx, networkId, params,
			"application/json", bytes.NewReader(encoded))
		if err != nil {
			lastErr = err
			continue
		}
		if response.JSON201 != nil && response.JSON201.RuleId != nil {
			return *response.JSON201.RuleId, nil
		}
		lastErr = errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		if !api.AnswerLost(response.HTTPResponse.StatusCode) {
			return "", lastErr
		}
	}
	return "", lastErr
}
