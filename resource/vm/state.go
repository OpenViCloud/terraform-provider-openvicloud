// Package vm is the openvicloud_vm resource: a virtual machine of the tenant on Incus. Power (stop, start) is not
// managed here: Terraform declares what exists, the CLI and portal turn a VM on and off.
package vm

import (
	"fmt"
	"net/http"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Created reports whether a create is done: Incus says Running or Ready and no request is pending. A state word the
// provider does not know keeps it waiting: the API may add words without breaking /v1.
func Created(v api.VmQueryResult) (bool, error) {
	if v.Transition != nil {
		return false, nil
	}
	if v.RequestFailureReason != nil {
		return false, fmt.Errorf("the VM was not created: %s", *v.RequestFailureReason)
	}
	if v.CurrentExistence != nil && *v.CurrentExistence == api.CurrentExistenceNotFound {
		return false, fmt.Errorf("the VM was not created: Incus no longer has its instance")
	}
	if v.CurrentState == nil {
		return false, nil
	}
	switch *v.CurrentState {
	case api.VmStateRunning, api.VmStateReady:
		return v.CurrentExistence != nil && *v.CurrentExistence == api.CurrentExistenceExists, nil
	case api.VmStateError:
		return false, fmt.Errorf("the VM was not created: Incus reports the instance in state Error")
	}
	return false, nil
}

// Gone reports whether a VM is gone: 404, or the API keeps the row as Deleted. A VM delete that does not go through
// keeps its request pending, so it ends in the wait's timeout, not here.
func Gone(status int, v *api.VmQueryResult) (bool, error) {
	if status == http.StatusNotFound {
		return true, nil
	}
	if v == nil {
		return false, nil
	}
	return v.CurrentExistence != nil && *v.CurrentExistence == api.CurrentExistenceDeleted, nil
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A delete wins over
// a pending stop or start, and repeats a pending delete; a request the provider does not know is waited out.
func Deletable(v api.VmQueryResult) bool {
	if v.Transition == nil {
		return true
	}
	switch *v.Transition {
	case api.VmTransitionStopping, api.VmTransitionStarting, api.VmTransitionDeleting:
		return true
	}
	return false
}
