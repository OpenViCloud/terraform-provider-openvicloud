package network

import (
	"context"
	"errors"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

const createAttempts = 3

// sendCreate sends the create, again with the same Idempotency-Key while its answer is lost, and returns the new
// network's id.
func sendCreate(ctx context.Context, client *api.ClientWithResponses, body api.CreateNetworkCommand, sleep func(time.Duration)) (string, error) {
	params := &api.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointParams{IdempotencyKey: api.NewIdempotencyKey()}
	var lastErr error
	for attempt := 1; attempt <= createAttempts; attempt++ {
		if attempt > 1 {
			sleep(2 * time.Second)
		}
		response, err := client.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointWithResponse(ctx, params, body)
		if err != nil {
			lastErr = err
			continue
		}
		if response.JSON202 != nil && response.JSON202.NetworkId != nil {
			return *response.JSON202.NetworkId, nil
		}
		lastErr = errors.New(api.Explain(api.ProblemOf(response.HTTPResponse, response.Body)))
		if !api.AnswerLost(response.StatusCode()) {
			return "", lastErr
		}
	}
	return "", lastErr
}
