package network

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aNetwork(existence api.CurrentExistence, state *api.NetworkStatus, transition *api.NetworkTransition) api.NetworkQueryResult {
	return api.NetworkQueryResult{CurrentExistence: &existence, CurrentState: state, Transition: transition,
		DesiredExistence: ptr(api.DesiredExistenceExists)}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	done, err := Created(aNetwork(api.CurrentExistenceNotCreated, nil, ptr(api.NetworkTransitionCreating)))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_IncusSaysCreated_IsDone(t *testing.T) {
	done, err := Created(aNetwork(api.CurrentExistenceExists, ptr(api.NetworkStatusCreated), nil))
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_AStateTheProviderDoesNotKnow_KeepsWaiting(t *testing.T) {
	done, err := Created(aNetwork(api.CurrentExistenceExists, ptr(api.NetworkStatus("Rebuilding")), nil))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	n := aNetwork(api.CurrentExistenceNotCreated, nil, nil)
	n.RequestFailureReason = ptr("Incus refused the network")

	done, err := Created(n)
	if done || err == nil || !strings.Contains(err.Error(), "Incus refused the network") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestGone_NotFoundOrDeleted_IsGone(t *testing.T) {
	if done, err := Gone(404, nil); !done || err != nil {
		t.Fatalf("404: done = %v, err = %v", done, err)
	}
	deleted := aNetwork(api.CurrentExistenceDeleted, nil, nil)
	if done, err := Gone(200, &deleted); !done || err != nil {
		t.Fatalf("Deleted: done = %v, err = %v", done, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	n := aNetwork(api.CurrentExistenceExists, ptr(api.NetworkStatusCreated), ptr(api.NetworkTransitionDeleting))
	n.DesiredExistence = ptr(api.DesiredExistenceDeleted)

	if done, err := Gone(200, &n); done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestGone_DeleteGivenUp_FailsWithTheReason(t *testing.T) {
	n := aNetwork(api.CurrentExistenceExists, ptr(api.NetworkStatusCreated), nil)
	n.RequestFailureReason = ptr("Incus refused the delete")

	done, err := Gone(200, &n)
	if done || err == nil || !strings.Contains(err.Error(), "Incus refused the delete") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}
