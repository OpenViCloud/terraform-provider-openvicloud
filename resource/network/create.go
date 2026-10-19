package network

import (
	"context"
	"net/http"
	"time"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func sendCreate(ctx context.Context, client *api.ClientWithResponses, body api.CreateNetworkCommand, sleep func(time.Duration)) (string, error) {
	return api.SendCreate(sleep, func(key string) (*string, *http.Response, []byte, error) {
		params := &api.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointParams{IdempotencyKey: key}
		response, err := client.TitanInfrastructureEndpointsNetworkCreateNetworkEndpointWithResponse(ctx, params, body)
		if err != nil {
			return nil, nil, nil, err
		}
		var id *string
		if response.JSON202 != nil {
			id = response.JSON202.NetworkId
		}
		return id, response.HTTPResponse, response.Body, nil
	})
}
