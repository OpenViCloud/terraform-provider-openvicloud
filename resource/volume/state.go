// Package volume is the openvicloud_volume resource: a block volume of the tenant on Incus. The API has no GET of one
// volume, so every read goes through the list.
package volume

import (
	"errors"
	"fmt"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Find is the volume with this id in the list, or nil.
func Find(list []api.VolumeQueryResult, id string) *api.VolumeQueryResult {
	for _, v := range list {
		if v.Id != nil && *v.Id == id {
			found := v
			return &found
		}
	}
	return nil
}

// Created reports whether a create is done: the volume exists on Incus and no request is pending. A create Incus gave
// up on keeps the volume listed with the reason set; it leaves the list only when someone deleted it, so a volume that
// left the list was not created.
func Created(v *api.VolumeQueryResult) (bool, error) {
	if v == nil {
		return false, errors.New("the volume was not created: it is no longer listed")
	}
	if v.Transition != nil {
		return false, nil
	}
	if v.RequestFailureReason != nil {
		return false, fmt.Errorf("the volume was not created: %s", *v.RequestFailureReason)
	}
	return v.CurrentExistence != nil && *v.CurrentExistence == api.CurrentExistenceExists, nil
}

// Gone reports whether a volume is gone: it left the list, or the API still lists it as Deleted. A delete Incus
// refused puts the desired existence back to Exists, with nothing pending and the reason set.
func Gone(v *api.VolumeQueryResult) (bool, error) {
	if v == nil {
		return true, nil
	}
	if v.CurrentExistence != nil && *v.CurrentExistence == api.CurrentExistenceDeleted {
		return true, nil
	}
	refused := v.Transition == nil && v.DesiredExistence != nil && *v.DesiredExistence == api.DesiredExistenceExists &&
		v.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the volume was not deleted: %s", *v.RequestFailureReason)
	}
	return false, nil
}

// Attached reports whether an attach to vmId is done: Incus shows the volume on that VM. An attach given up puts the
// desired VM back to the current one, with the reason set.
func Attached(v *api.VolumeQueryResult, vmId string) (bool, error) {
	if v == nil {
		return false, errors.New("the volume is no longer listed")
	}
	if v.Transition != nil {
		return false, nil
	}
	if v.CurrentAttachedVmId != nil && *v.CurrentAttachedVmId == vmId {
		return true, nil
	}
	if v.RequestFailureReason != nil {
		return false, fmt.Errorf("the volume was not attached: %s", *v.RequestFailureReason)
	}
	return false, nil
}

// Detached reports whether a detach is done: Incus shows the volume on no VM, or the volume is gone.
func Detached(v *api.VolumeQueryResult) (bool, error) {
	if v == nil {
		return true, nil
	}
	if v.Transition != nil {
		return false, nil
	}
	if v.CurrentAttachedVmId == nil {
		return true, nil
	}
	if v.RequestFailureReason != nil {
		return false, fmt.Errorf("the volume was not detached: %s", *v.RequestFailureReason)
	}
	return false, nil
}

// Deletable reports whether the API takes a delete now: it refuses one while another request (create, attach, detach)
// is pending. A pending delete is repeated; a volume no longer listed needs no wait.
func Deletable(v *api.VolumeQueryResult) bool {
	return v == nil || v.Transition == nil || *v.Transition == api.VolumeTransitionDeleting
}

// Detachable reports whether the API takes a detach now: it refuses one while another request is pending. A pending
// detach is repeated; a volume no longer listed needs no wait.
func Detachable(v *api.VolumeQueryResult) bool {
	return v == nil || v.Transition == nil || *v.Transition == api.VolumeTransitionDetaching
}
