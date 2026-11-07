// Package network is the openvicloud_network resource: a private network of the tenant on Incus (OVN).
package network

import (
	"fmt"
	"net/http"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Created reports whether a create is done: the status is Created. A new network carries no failure reason until its
// create gives up, so a reason fails the create whatever the status. Failed and Deleted fail it; Creating, Pending,
// Unknown and a status word the provider does not know keep it waiting: the API may add words without breaking /v1.
func Created(n api.NetworkQueryResult) (bool, error) {
	if n.Status == nil {
		return false, nil
	}
	if n.RequestFailureReason != nil {
		return false, fmt.Errorf("the network was not created, it is %s: %s", *n.Status, *n.RequestFailureReason)
	}
	switch *n.Status {
	case api.NetworkStatusCreated:
		return true, nil
	case api.NetworkStatusFailed:
		return false, fmt.Errorf("the network was not created, it is Failed")
	case api.NetworkStatusDeleted:
		return false, fmt.Errorf("the network was deleted before its create finished")
	}
	return false, nil
}

// Gone reports whether a network is gone: 404, or the API keeps the row as Deleted. A delete Incus refused returns the
// network to the status it had, with the reason set; Deleting, and Creating, are waited out.
func Gone(status int, n *api.NetworkQueryResult) (bool, error) {
	if status == http.StatusNotFound {
		return true, nil
	}
	if n == nil || n.Status == nil {
		return false, nil
	}
	if *n.Status == api.NetworkStatusDeleted {
		return true, nil
	}
	refused := *n.Status != api.NetworkStatusDeleting && *n.Status != api.NetworkStatusCreating && n.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the network was not deleted, it is %s: %s", *n.Status, *n.RequestFailureReason)
	}
	return false, nil
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A pending delete is
// repeated; a network not found needs no wait.
func Deletable(n *api.NetworkQueryResult) bool {
	return n == nil || n.Status == nil || *n.Status != api.NetworkStatusCreating
}
