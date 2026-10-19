// Package network is the openvicloud_network resource: a private network of the tenant on Incus (OVN).
package network

import (
	"fmt"
	"net/http"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Created reports whether a create is done: Incus says Created and no request is pending. A state word the provider
// does not know keeps it waiting: the API may add words without breaking /v1.
func Created(n api.NetworkQueryResult) (bool, error) {
	if n.Transition != nil {
		return false, nil
	}
	if n.RequestFailureReason != nil {
		return false, fmt.Errorf("the network was not created: %s", *n.RequestFailureReason)
	}
	exists := n.CurrentExistence != nil && *n.CurrentExistence == api.CurrentExistenceExists
	created := n.CurrentState != nil && *n.CurrentState == api.NetworkStatusCreated
	return exists && created, nil
}

// Gone reports whether a network is gone: 404, or the API keeps the row as Deleted. A delete Incus refused puts the
// desired existence back to Exists, with nothing pending and the reason set.
func Gone(status int, n *api.NetworkQueryResult) (bool, error) {
	if status == http.StatusNotFound {
		return true, nil
	}
	if n == nil {
		return false, nil
	}
	if n.CurrentExistence != nil && *n.CurrentExistence == api.CurrentExistenceDeleted {
		return true, nil
	}
	refused := n.Transition == nil && n.DesiredExistence != nil && *n.DesiredExistence == api.DesiredExistenceExists &&
		n.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the network was not deleted: %s", *n.RequestFailureReason)
	}
	return false, nil
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A pending delete is
// repeated; a network not found needs no wait.
func Deletable(n *api.NetworkQueryResult) bool {
	return n == nil || n.Transition == nil || *n.Transition == api.NetworkTransitionDeleting
}
