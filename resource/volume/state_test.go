package volume

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aVolume(id string, status api.VolumeStatus) api.VolumeQueryResult {
	return api.VolumeQueryResult{Id: &id, Status: &status}
}

func TestFind_ById(t *testing.T) {
	list := []api.VolumeQueryResult{aVolume("a", api.VolumeStatusAvailable), aVolume("b", api.VolumeStatusAvailable)}
	if found := Find(list, "b"); found == nil || *found.Id != "b" {
		t.Fatalf("found = %v", found)
	}
	if Find(list, "c") != nil {
		t.Fatal("c is not in the list")
	}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.VolumeStatusCreating)
	done, err := Created(&v)
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_Exists_IsDone(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAvailable)
	done, err := Created(&v)
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.VolumeStatusFailed)
	v.RequestFailureReason = ptr("pool full")
	_, err := Created(&v)
	if err == nil || !strings.Contains(err.Error(), "pool full") {
		t.Fatalf("err = %v", err)
	}
}

// Someone deleted it while the create was still waiting: the volume leaves the list.
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
	v := aVolume("a", api.VolumeStatusDeleting)
	gone, err := Gone(&v)
	if gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_DeleteRefused_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAvailable)
	v.RequestFailureReason = ptr("volume busy")
	_, err := Gone(&v)
	if err == nil || !strings.Contains(err.Error(), "volume busy") {
		t.Fatalf("err = %v", err)
	}
}

func TestAttached_ToTheVm_IsDone(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAvailable)
	v.CurrentAttachedVmId = ptr("vm-1")
	done, err := Attached(&v, "vm-1")
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestAttached_WhileAttaching_KeepsWaiting(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAttaching)
	done, err := Attached(&v, "vm-1")
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestAttached_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAvailable)
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
	v := aVolume("a", api.VolumeStatusAvailable)
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
	v := aVolume("a", api.VolumeStatusDetaching)
	v.CurrentAttachedVmId = ptr("vm-1")
	done, err := Detached(&v)
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestDetached_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAvailable)
	v.CurrentAttachedVmId = ptr("vm-1")
	v.RequestFailureReason = ptr("device busy")
	_, err := Detached(&v)
	if err == nil || !strings.Contains(err.Error(), "device busy") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletable_AtRestOrDeleting_Goes(t *testing.T) {
	for _, status := range []api.VolumeStatus{api.VolumeStatusAvailable, api.VolumeStatusNotFound, api.VolumeStatusFailed, api.VolumeStatusDeleting} {
		v := aVolume("a", status)
		if !Deletable(&v) {
			t.Fatalf("status %v", status)
		}
	}
	if !Deletable(nil) {
		t.Fatal("a volume no longer listed needs no wait")
	}
}

func TestDeletable_AnotherRequestPending_Waits(t *testing.T) {
	for _, status := range []api.VolumeStatus{api.VolumeStatusCreating, api.VolumeStatusAttaching, api.VolumeStatusDetaching} {
		v := aVolume("a", status)
		if Deletable(&v) {
			t.Fatalf("the API refuses to delete a volume while %s", status)
		}
	}
}

func TestDetachable_AtRestOrDetaching_Goes(t *testing.T) {
	for _, status := range []api.VolumeStatus{api.VolumeStatusAvailable, api.VolumeStatusDetaching} {
		v := aVolume("a", status)
		if !Detachable(&v) {
			t.Fatalf("status %v", status)
		}
	}
	if !Detachable(nil) {
		t.Fatal("a volume no longer listed needs no wait")
	}
}

func TestDetachable_WhileAttaching_Waits(t *testing.T) {
	v := aVolume("a", api.VolumeStatusAttaching)
	if Detachable(&v) {
		t.Fatal("the API refuses to detach a volume whose attach is pending")
	}
}

func TestCreated_FailedWithoutAReason_StillFails(t *testing.T) {
	v := aVolume("a", api.VolumeStatusFailed)
	if _, err := Created(&v); err == nil {
		t.Fatal("a failed create is not created, reason or not")
	}
}

func TestGone_Deleted_IsGone(t *testing.T) {
	v := aVolume("a", api.VolumeStatusDeleted)
	gone, err := Gone(&v)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}
