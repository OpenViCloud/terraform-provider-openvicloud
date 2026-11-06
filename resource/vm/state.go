// Package vm is the openvicloud_vm resource: a virtual machine of the tenant on Incus. Power (stop, start) is not
// managed here: Terraform declares what exists, the CLI and portal turn a VM on and off.
package vm

import (
	"fmt"
	"net/http"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// atRest is every status in which no request waits on the VM and Incus has said what its instance is doing. Unknown
// is not: the next observation replaces it.
func atRest(status api.VmStatus) bool {
	switch status {
	case api.VmStatusRunning, api.VmStatusReady, api.VmStatusStopped, api.VmStatusFrozen, api.VmStatusError:
		return true
	}
	return false
}

// Created reports whether a create is done: Incus says Running or Ready. Unknown, Stopped or Frozen without a failure
// reason, and a status word the provider does not know keep it waiting: the API may add words without breaking /v1.
func Created(v api.VmQueryResult) (bool, error) {
	if v.Status == nil {
		return false, nil
	}
	switch *v.Status {
	case api.VmStatusRunning, api.VmStatusReady:
		return true, nil
	case api.VmStatusFailed, api.VmStatusError:
		return false, fmt.Errorf("the VM was not created, it is %s: %s", *v.Status, reason(v))
	case api.VmStatusNotFound:
		return false, fmt.Errorf("the VM was not created: Incus no longer has its instance")
	case api.VmStatusDeleted:
		return false, fmt.Errorf("the VM was deleted before its create finished")
	case api.VmStatusStopped, api.VmStatusFrozen:
		// A create that gave up after Incus made the instance rests at the Incus word, with the reason.
		if v.RequestFailureReason != nil {
			return false, fmt.Errorf("the VM was not created, it is %s: %s", *v.Status, *v.RequestFailureReason)
		}
	}
	return false, nil
}

// Gone reports whether a VM is gone: 404, or the API still answers with status Deleted. A delete that ends in any
// other settled status did not go through; Unknown is waited out like a pending request.
func Gone(code int, v *api.VmQueryResult) (bool, error) {
	if code == http.StatusNotFound {
		return true, nil
	}
	if v == nil || v.Status == nil {
		return false, nil
	}
	status := *v.Status
	if status == api.VmStatusDeleted {
		return true, nil
	}
	if atRest(status) || status == api.VmStatusNotFound || status == api.VmStatusFailed {
		return false, fmt.Errorf("the VM was not deleted, it is %s: %s", status, reason(*v))
	}
	return false, nil
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A delete wins over
// a pending stop or start, and repeats a pending delete; a status the provider does not know is waited out.
func Deletable(v api.VmQueryResult) bool {
	if v.Status == nil {
		return false
	}
	status := *v.Status
	switch {
	case atRest(status):
		return true
	case status == api.VmStatusUnknown, status == api.VmStatusStarting, status == api.VmStatusStopping,
		status == api.VmStatusDeleting, status == api.VmStatusNotFound, status == api.VmStatusFailed, status == api.VmStatusDeleted:
		return true
	}
	return false
}

func reason(v api.VmQueryResult) string {
	if v.RequestFailureReason == nil {
		return "the API gives no reason"
	}
	return *v.RequestFailureReason
}
