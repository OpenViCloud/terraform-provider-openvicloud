package vm

import (
	"net/http"
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aVm(existence api.CurrentExistence, state *api.VmState, transition *api.VmTransition) api.VmQueryResult {
	return api.VmQueryResult{CurrentExistence: &existence, CurrentState: state, Transition: transition,
		DesiredExistence: ptr(api.DesiredExistenceExists)}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	done, err := Created(aVm(api.CurrentExistenceNotCreated, nil, ptr(api.VmTransitionCreating)))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RunningOrReady_IsDone(t *testing.T) {
	for _, state := range []api.VmState{api.VmStateRunning, api.VmStateReady} {
		done, err := Created(aVm(api.CurrentExistenceExists, ptr(state), nil))
		if !done || err != nil {
			t.Fatalf("%s: done = %v, err = %v", state, done, err)
		}
	}
}

func TestCreated_AStateTheProviderDoesNotKnow_KeepsWaiting(t *testing.T) {
	done, err := Created(aVm(api.CurrentExistenceExists, ptr(api.VmState("Migrating")), nil))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	v := aVm(api.CurrentExistenceNotCreated, nil, nil)
	v.RequestFailureReason = ptr("template not on the cluster")
	_, err := Created(v)
	if err == nil || !strings.Contains(err.Error(), "template not on the cluster") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreated_IncusSaysError_Fails(t *testing.T) {
	_, err := Created(aVm(api.CurrentExistenceExists, ptr(api.VmStateError), nil))
	if err == nil || !strings.Contains(err.Error(), "Error") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreated_InstanceVanished_Fails(t *testing.T) {
	_, err := Created(aVm(api.CurrentExistenceNotFound, nil, nil))
	if err == nil {
		t.Fatal("a VM Incus no longer has is not created")
	}
}

func TestGone_NotFound_IsGone(t *testing.T) {
	gone, err := Gone(http.StatusNotFound, nil)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_ExistenceDeleted_IsGone(t *testing.T) {
	v := aVm(api.CurrentExistenceDeleted, nil, nil)
	gone, err := Gone(http.StatusOK, &v)
	if !gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	v := aVm(api.CurrentExistenceExists, ptr(api.VmStateRunning), ptr(api.VmTransitionDeleting))
	gone, err := Gone(http.StatusOK, &v)
	if gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestDeletable_WhileCreating_Waits(t *testing.T) {
	if Deletable(aVm(api.CurrentExistenceNotCreated, nil, ptr(api.VmTransitionCreating))) {
		t.Fatal("the API refuses to delete a VM whose create is pending")
	}
}

func TestDeletable_AtRestOrStoppingOrDeleting_Goes(t *testing.T) {
	for _, transition := range []*api.VmTransition{nil, ptr(api.VmTransitionStopping), ptr(api.VmTransitionStarting), ptr(api.VmTransitionDeleting)} {
		if !Deletable(aVm(api.CurrentExistenceExists, ptr(api.VmStateRunning), transition)) {
			t.Fatalf("transition %v: a delete wins over stop and start, and repeats a pending delete", transition)
		}
	}
}

func TestDeletable_ATransitionTheProviderDoesNotKnow_Waits(t *testing.T) {
	if Deletable(aVm(api.CurrentExistenceExists, ptr(api.VmStateRunning), ptr(api.VmTransition("Migrating")))) {
		t.Fatal("an unknown request may refuse the delete; wait it out")
	}
}
