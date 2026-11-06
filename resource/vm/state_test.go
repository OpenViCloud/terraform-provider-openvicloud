package vm

import (
	"net/http"
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aVm(status api.VmStatus) api.VmQueryResult {
	return api.VmQueryResult{Status: &status}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	done, err := Created(aVm(api.VmStatusCreating))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RunningOrReady_IsDone(t *testing.T) {
	for _, status := range []api.VmStatus{api.VmStatusRunning, api.VmStatusReady} {
		done, err := Created(aVm(status))
		if !done || err != nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

func TestCreated_UnknownStoppedOrFrozen_KeepsWaiting(t *testing.T) {
	for _, status := range []api.VmStatus{api.VmStatusUnknown, api.VmStatusStopped, api.VmStatusFrozen} {
		done, err := Created(aVm(status))
		if done || err != nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

// A snapshot create or revert runs on the instance: the VM returns to the Incus word it held, so a create that
// polls past it is not done and not failed.
func TestCreated_WhileASnapshotRuns_KeepsWaiting(t *testing.T) {
	done, err := Created(aVm(api.VmStatusSnapshotting))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_AStateTheProviderDoesNotKnow_KeepsWaiting(t *testing.T) {
	done, err := Created(aVm(api.VmStatus("Migrating")))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVm(api.VmStatusFailed)
	v.RequestFailureReason = ptr("template not on the cluster")
	_, err := Created(v)
	if err == nil || !strings.Contains(err.Error(), "template not on the cluster") {
		t.Fatalf("err = %v", err)
	}
}

// The instance exists, so the VM goes on from Unknown to whatever the reconciler reads, Running included.
func TestCreated_GivenUpAfterTheInstanceExists_FailsWithTheReason_ThoughItRunsLater(t *testing.T) {
	for _, status := range []api.VmStatus{api.VmStatusUnknown, api.VmStatusStopped, api.VmStatusRunning} {
		v := aVm(status)
		v.RequestFailureReason = ptr("cloud-init never reported")
		_, err := Created(v)
		if err == nil || !strings.Contains(err.Error(), "cloud-init never reported") {
			t.Fatalf("%s: err = %v", status, err)
		}
	}
}

func TestCreated_IncusSaysError_Fails(t *testing.T) {
	_, err := Created(aVm(api.VmStatusError))
	if err == nil || !strings.Contains(err.Error(), "Error") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreated_InstanceVanished_Fails(t *testing.T) {
	_, err := Created(aVm(api.VmStatusNotFound))
	if err == nil {
		t.Fatal("a VM Incus no longer has is not created")
	}
}

func TestCreated_DeletedBeforeTheCreateFinished_Fails(t *testing.T) {
	_, err := Created(aVm(api.VmStatusDeleted))
	if err == nil {
		t.Fatal("a deleted VM is not created")
	}
}

func TestGone_NotFound_IsGone(t *testing.T) {
	gone, err := Gone(http.StatusNotFound, nil)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_StatusDeleted_IsGone(t *testing.T) {
	v := aVm(api.VmStatusDeleted)
	gone, err := Gone(http.StatusOK, &v)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	for _, status := range []api.VmStatus{api.VmStatusDeleting, api.VmStatusUnknown, api.VmStatusSnapshotting, api.VmStatus("Migrating")} {
		v := aVm(status)
		gone, err := Gone(http.StatusOK, &v)
		if gone || err != nil {
			t.Fatalf("%s: gone = %v, err = %v", status, gone, err)
		}
	}
}

func TestGone_DeleteGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVm(api.VmStatusRunning)
	v.RequestFailureReason = ptr("Incus refused to delete the instance")
	_, err := Gone(http.StatusOK, &v)
	if err == nil || !strings.Contains(err.Error(), "Incus refused to delete the instance") || !strings.Contains(err.Error(), "Running") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeletable_WhileCreating_Waits(t *testing.T) {
	if Deletable(aVm(api.VmStatusCreating)) {
		t.Fatal("the API refuses to delete a VM whose create is pending")
	}
}

func TestDeletable_AtRestOrStoppingOrDeleting_Goes(t *testing.T) {
	for _, status := range []api.VmStatus{api.VmStatusRunning, api.VmStatusStopped, api.VmStatusUnknown, api.VmStatusNotFound,
		api.VmStatusFailed, api.VmStatusStopping, api.VmStatusStarting, api.VmStatusDeleting} {
		if !Deletable(aVm(status)) {
			t.Fatalf("%s: a delete wins over stop and start, repeats a pending delete, and removes a failed create", status)
		}
	}
}

// The API refuses a delete while a snapshot create or revert runs (VM_INVALID_STATUS): the provider waits for it.
func TestDeletable_WhileASnapshotRuns_Waits(t *testing.T) {
	if Deletable(aVm(api.VmStatusSnapshotting)) {
		t.Fatal("the API refuses to delete a VM while a snapshot create or revert runs")
	}
}

func TestDeletable_AStatusTheProviderDoesNotKnow_Waits(t *testing.T) {
	if Deletable(aVm(api.VmStatus("Migrating"))) {
		t.Fatal("an unknown request may refuse the delete; wait it out")
	}
}
