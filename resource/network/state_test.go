package network

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aNetwork(status api.NetworkStatus) api.NetworkQueryResult {
	return api.NetworkQueryResult{Status: &status}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	done, err := Created(aNetwork(api.NetworkStatusCreating))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_IncusSaysCreated_IsDone(t *testing.T) {
	done, err := Created(aNetwork(api.NetworkStatusCreated))
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_AStatusTheProviderDoesNotKnow_KeepsWaiting(t *testing.T) {
	for _, status := range []api.NetworkStatus{"Rebuilding", api.NetworkStatusPending, api.NetworkStatusUnknown} {
		done, err := Created(aNetwork(status))
		if done || err != nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	n := aNetwork(api.NetworkStatusFailed)
	n.RequestFailureReason = ptr("Incus refused the network")

	done, err := Created(n)
	if done || err == nil || !strings.Contains(err.Error(), "Incus refused the network") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_FailedWithoutAReason_OrDeleted_Fails(t *testing.T) {
	for _, status := range []api.NetworkStatus{api.NetworkStatusFailed, api.NetworkStatusDeleted} {
		done, err := Created(aNetwork(status))
		if done || err == nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

func TestGone_NotFoundOrDeleted_IsGone(t *testing.T) {
	if done, err := Gone(404, nil); !done || err != nil {
		t.Fatalf("404: done = %v, err = %v", done, err)
	}
	deleted := aNetwork(api.NetworkStatusDeleted)
	if done, err := Gone(200, &deleted); !done || err != nil {
		t.Fatalf("Deleted: done = %v, err = %v", done, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	n := aNetwork(api.NetworkStatusDeleting)

	if done, err := Gone(200, &n); done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestGone_DeleteGivenUp_FailsWithTheReason(t *testing.T) {
	n := aNetwork(api.NetworkStatusCreated)
	n.RequestFailureReason = ptr("Incus refused the delete")

	done, err := Gone(200, &n)
	if done || err == nil || !strings.Contains(err.Error(), "Incus refused the delete") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestDeletable_AtRestOrDeleting_Goes(t *testing.T) {
	for _, status := range []api.NetworkStatus{api.NetworkStatusCreated, api.NetworkStatusDeleting, api.NetworkStatusFailed} {
		n := aNetwork(status)
		if !Deletable(&n) {
			t.Fatalf("status %s", status)
		}
	}
	if !Deletable(nil) {
		t.Fatal("a network not found needs no wait")
	}
}

func TestDeletable_WhileCreating_Waits(t *testing.T) {
	n := aNetwork(api.NetworkStatusCreating)
	if Deletable(&n) {
		t.Fatal("the API refuses to delete a network whose create is pending")
	}
}
