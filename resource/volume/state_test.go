package volume

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aVolume(id string, existence api.CurrentExistence, transition *api.VolumeTransition) api.VolumeQueryResult {
	return api.VolumeQueryResult{Id: &id, CurrentExistence: &existence, Transition: transition,
		DesiredExistence: ptr(api.DesiredExistenceExists)}
}

func TestFind_ById(t *testing.T) {
	list := []api.VolumeQueryResult{aVolume("a", api.CurrentExistenceExists, nil), aVolume("b", api.CurrentExistenceExists, nil)}
	if found := Find(list, "b"); found == nil || *found.Id != "b" {
		t.Fatalf("found = %v", found)
	}
	if Find(list, "c") != nil {
		t.Fatal("c is not in the list")
	}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceNotCreated, ptr(api.VolumeTransitionCreating))
	done, err := Created(&v)
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_Exists_IsDone(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	done, err := Created(&v)
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceNotCreated, nil)
	v.RequestFailureReason = ptr("pool full")
	_, err := Created(&v)
	if err == nil || !strings.Contains(err.Error(), "pool full") {
		t.Fatalf("err = %v", err)
	}
}

// A create Incus refused before the volume had an id is deleted on the spot: the volume leaves the list.
func TestCreated_LeftTheList_Fails(t *testing.T) {
	_, err := Created(nil)
	if err == nil {
		t.Fatal("a volume no longer listed is not created")
	}
}

func TestGone_LeftTheList_IsGone(t *testing.T) {
	gone, err := Gone(nil)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, ptr(api.VolumeTransitionDeleting))
	gone, err := Gone(&v)
	if gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_DeleteRefused_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	v.RequestFailureReason = ptr("volume busy")
	_, err := Gone(&v)
	if err == nil || !strings.Contains(err.Error(), "volume busy") {
		t.Fatalf("err = %v", err)
	}
}

func TestAttached_ToTheVm_IsDone(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	v.CurrentAttachedVmId = ptr("vm-1")
	done, err := Attached(&v, "vm-1")
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestAttached_WhileAttaching_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, ptr(api.VolumeTransitionAttaching))
	done, err := Attached(&v, "vm-1")
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestAttached_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	v.RequestFailureReason = ptr("VM is stopping")
	_, err := Attached(&v, "vm-1")
	if err == nil || !strings.Contains(err.Error(), "VM is stopping") {
		t.Fatalf("err = %v", err)
	}
}

func TestAttached_VolumeLeftTheList_Fails(t *testing.T) {
	if _, err := Attached(nil, "vm-1"); err == nil {
		t.Fatal("a volume no longer listed cannot be attached")
	}
}

func TestDetached_NoVm_IsDone(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	done, err := Detached(&v)
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestDetached_VolumeGone_IsDone(t *testing.T) {
	done, err := Detached(nil)
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestDetached_WhileDetaching_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, ptr(api.VolumeTransitionDetaching))
	v.CurrentAttachedVmId = ptr("vm-1")
	done, err := Detached(&v)
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestDetached_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, nil)
	v.CurrentAttachedVmId = ptr("vm-1")
	v.RequestFailureReason = ptr("device busy")
	_, err := Detached(&v)
	if err == nil || !strings.Contains(err.Error(), "device busy") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletable_AtRestOrDeleting_Goes(t *testing.T) {
	for _, transition := range []*api.VolumeTransition{nil, ptr(api.VolumeTransitionDeleting)} {
		v := aVolume("a", api.CurrentExistenceExists, transition)
		if !Deletable(&v) {
			t.Fatalf("transition %v", transition)
		}
	}
	if !Deletable(nil) {
		t.Fatal("a volume no longer listed needs no wait")
	}
}

func TestDeletable_AnotherRequestPending_Waits(t *testing.T) {
	for _, transition := range []api.VolumeTransition{api.VolumeTransitionCreating, api.VolumeTransitionAttaching, api.VolumeTransitionDetaching} {
		v := aVolume("a", api.CurrentExistenceExists, ptr(transition))
		if Deletable(&v) {
			t.Fatalf("the API refuses to delete a volume while %s", transition)
		}
	}
}

func TestDetachable_AtRestOrDetaching_Goes(t *testing.T) {
	for _, transition := range []*api.VolumeTransition{nil, ptr(api.VolumeTransitionDetaching)} {
		v := aVolume("a", api.CurrentExistenceExists, transition)
		if !Detachable(&v) {
			t.Fatalf("transition %v", transition)
		}
	}
	if !Detachable(nil) {
		t.Fatal("a volume no longer listed needs no wait")
	}
}

func TestDetachable_WhileAttaching_Waits(t *testing.T) {
	v := aVolume("a", api.CurrentExistenceExists, ptr(api.VolumeTransitionAttaching))
	if Detachable(&v) {
		t.Fatal("the API refuses to detach a volume whose attach is pending")
	}
}
