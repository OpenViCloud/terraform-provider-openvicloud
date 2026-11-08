package loadbalancer

import (
	"net/http"
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aLoadBalancer(status api.LoadBalancerStatus) api.LoadBalancerQueryResult {
	return api.LoadBalancerQueryResult{Status: &status,
		Listeners: &[]api.LoadBalancerListenerQueryResult{{Protocol: ptr("TCP"), Port: ptr(int32(80)), TargetPort: ptr(int32(8080))}},
		Backends:  &[]api.LoadBalancerBackendQueryResult{{VmId: ptr("vm-a")}, {VmId: ptr("vm-b")}}}
}

func TestCreated_WhileCreating_KeepsWaiting(t *testing.T) {
	done, err := Created(aLoadBalancer(api.LoadBalancerStatusCreating))
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_OnIncus_IsDone(t *testing.T) {
	done, err := Created(aLoadBalancer(api.LoadBalancerStatusAvailable))
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusFailed)
	lb.RequestFailureReason = ptr("no free VIP")
	_, err := Created(lb)
	if err == nil || !strings.Contains(err.Error(), "no free VIP") {
		t.Fatalf("err = %v", err)
	}
}

// Someone deleted it while the create was still waiting.
func TestCreated_DeletedBeforeTheCreateFinished_Fails(t *testing.T) {
	_, err := Created(aLoadBalancer(api.LoadBalancerStatusDeleted))
	if err == nil {
		t.Fatal("a load balancer deleted before its create finished is not created")
	}
}

func TestCreated_AFailedCreateWithoutAReason_IsNotCreated(t *testing.T) {
	if _, err := Created(aLoadBalancer(api.LoadBalancerStatusFailed)); err == nil {
		t.Fatal("a create that gave up leaves the load balancer Failed")
	}
}

func TestCreated_WhileCreatingWithAnOlderReason_KeepsWaiting(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusCreating)
	lb.RequestFailureReason = ptr("an older request gave up")
	if done, err := Created(lb); done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestApplied_SameListenersAndBackendsInAnyOrder_IsTrue(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusAvailable)
	if !Applied(lb, []Listener{{"TCP", 80, 8080}}, []string{"vm-b", "vm-a"}) {
		t.Fatal("the same configuration in another order is applied")
	}
}

func TestApplied_ADifferentListenerOrBackend_IsFalse(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusAvailable)
	if Applied(lb, []Listener{{"TCP", 80, 9090}}, []string{"vm-a", "vm-b"}) {
		t.Fatal("another target port is not applied")
	}
	if Applied(lb, []Listener{{"TCP", 80, 8080}}, []string{"vm-a"}) {
		t.Fatal("another set of VMs is not applied")
	}
}

func TestUpdated_WhileUpdating_KeepsWaiting(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusUpdating)
	done, err := Updated(lb, []Listener{{"TCP", 80, 8080}}, []string{"vm-a", "vm-b"})
	if done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestUpdated_AtRestWithTheAskedConfiguration_IsDone(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusAvailable)
	done, err := Updated(lb, []Listener{{"TCP", 80, 8080}}, []string{"vm-a", "vm-b"})
	if !done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

// An update given up puts the configuration back to what Incus took, with the reason.
func TestUpdated_GivenUp_FailsWithTheReason(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusAvailable)
	lb.RequestFailureReason = ptr("Incus refused the listener")
	_, err := Updated(lb, []Listener{{"UDP", 53, 53}}, []string{"vm-a"})
	if err == nil || !strings.Contains(err.Error(), "Incus refused the listener") {
		t.Fatalf("err = %v", err)
	}
}

func TestSettled_OnlyWithNothingPending(t *testing.T) {
	for _, status := range []api.LoadBalancerStatus{api.LoadBalancerStatusAvailable, api.LoadBalancerStatusNotFound, api.LoadBalancerStatusFailed} {
		if !Settled(aLoadBalancer(status)) {
			t.Fatalf("%s is settled", status)
		}
	}
	for _, status := range []api.LoadBalancerStatus{api.LoadBalancerStatusCreating, api.LoadBalancerStatusUpdating, api.LoadBalancerStatusDeleting} {
		if Settled(aLoadBalancer(status)) {
			t.Fatalf("%s is not settled", status)
		}
	}
}

func TestDeletable_WhileCreating_Waits_ElseGoes(t *testing.T) {
	creating := aLoadBalancer(api.LoadBalancerStatusCreating)
	if Deletable(&creating) {
		t.Fatal("the API refuses to delete a load balancer whose create is pending")
	}
	for _, status := range []api.LoadBalancerStatus{api.LoadBalancerStatusAvailable, api.LoadBalancerStatusUpdating,
		api.LoadBalancerStatusDeleting, api.LoadBalancerStatusNotFound, api.LoadBalancerStatusFailed} {
		lb := aLoadBalancer(status)
		if !Deletable(&lb) {
			t.Fatalf("%s: a delete wins over an update, repeats a delete and takes a resting one", status)
		}
	}
	if !Deletable(nil) {
		t.Fatal("a load balancer not found needs no wait")
	}
}

func TestGone_NotFoundOrDeleted_IsGone(t *testing.T) {
	if gone, err := Gone(http.StatusNotFound, nil); !gone || err != nil {
		t.Fatalf("404: gone = %v, err = %v", gone, err)
	}
	deleted := aLoadBalancer(api.LoadBalancerStatusDeleted)
	if gone, err := Gone(http.StatusOK, &deleted); !gone || err != nil {
		t.Fatalf("Deleted: gone = %v, err = %v", gone, err)
	}
}

func TestGone_WhileDeleting_KeepsWaiting(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusDeleting)
	lb.RequestFailureReason = ptr("an older request gave up")
	if gone, err := Gone(http.StatusOK, &lb); gone || err != nil {
		t.Fatalf("gone = %v, err = %v", gone, err)
	}
}

func TestGone_DeleteRefused_FailsWithTheReason(t *testing.T) {
	lb := aLoadBalancer(api.LoadBalancerStatusAvailable)
	lb.RequestFailureReason = ptr("network busy")
	_, err := Gone(http.StatusOK, &lb)
	if err == nil || !strings.Contains(err.Error(), "network busy") {
		t.Fatalf("err = %v", err)
	}
}
