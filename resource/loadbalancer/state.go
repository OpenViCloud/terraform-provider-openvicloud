// Package loadbalancer is the openvicloud_load_balancer resource: an L4 load balancer on a network with public IPv6.
// Name changes in place (Titan-side), listeners and backends change in place through Incus; the VIP stays.
package loadbalancer

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Listener is one listener as the configuration declares it.
type Listener struct {
	Protocol   string
	Port       int32
	TargetPort int32
}

// waits reports whether a request waits on its command. A load balancer has one status; Creating, Updating and
// Deleting are the three that wait (titan-ops features/load-balancer/design.md#lb-lifecycle).
func waits(lb *api.LoadBalancerQueryResult) bool {
	if lb.Status == nil {
		return false
	}
	switch *lb.Status {
	case api.LoadBalancerStatusCreating, api.LoadBalancerStatusUpdating, api.LoadBalancerStatusDeleting:
		return true
	}
	return false
}

func hasStatus(lb *api.LoadBalancerQueryResult, status api.LoadBalancerStatus) bool {
	return lb.Status != nil && *lb.Status == status
}

// Created reports whether a create is done: Incus has the load balancer and nothing is pending. A create Incus gave up
// on leaves the load balancer Failed with the reason set; it is Deleted only when someone deleted it before the create
// finished.
func Created(lb api.LoadBalancerQueryResult) (bool, error) {
	if waits(&lb) {
		return false, nil
	}
	if lb.RequestFailureReason != nil {
		return false, fmt.Errorf("the load balancer was not created: %s", *lb.RequestFailureReason)
	}
	if hasStatus(&lb, api.LoadBalancerStatusFailed) {
		return false, errors.New("the load balancer was not created")
	}
	if hasStatus(&lb, api.LoadBalancerStatusDeleted) {
		return false, errors.New("the load balancer was not created: it was deleted before the create finished")
	}
	return hasStatus(&lb, api.LoadBalancerStatusAvailable), nil
}

// Applied reports whether the API holds exactly these listeners and backend VMs, in any order.
func Applied(lb api.LoadBalancerQueryResult, listeners []Listener, backendVmIds []string) bool {
	var have []Listener
	if lb.Listeners != nil {
		for _, l := range *lb.Listeners {
			have = append(have, Listener{Protocol: value(l.Protocol), Port: number(l.Port), TargetPort: number(l.TargetPort)})
		}
	}
	var haveVms []string
	if lb.Backends != nil {
		for _, b := range *lb.Backends {
			haveVms = append(haveVms, value(b.VmId))
		}
	}
	return sameListeners(have, listeners) && sameStrings(haveVms, backendVmIds)
}

// Updated reports whether an update is done: nothing pending and the API holds the asked configuration. An update
// given up puts the configuration back to what Incus took, with the reason.
func Updated(lb api.LoadBalancerQueryResult, listeners []Listener, backendVmIds []string) (bool, error) {
	if waits(&lb) {
		return false, nil
	}
	if Applied(lb, listeners, backendVmIds) {
		return true, nil
	}
	if lb.RequestFailureReason != nil {
		return false, fmt.Errorf("the load balancer was not updated: %s", *lb.RequestFailureReason)
	}
	return false, errors.New("the load balancer was not updated: OpenViCloud holds another configuration")
}

// Settled reports whether the API takes a new configuration now: it refuses one while another request is pending.
func Settled(lb api.LoadBalancerQueryResult) bool {
	return !waits(&lb)
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A delete wins over
// a pending update and repeats a pending delete; a load balancer not found needs no wait.
func Deletable(lb *api.LoadBalancerQueryResult) bool {
	if lb == nil {
		return true
	}
	return !hasStatus(lb, api.LoadBalancerStatusCreating)
}

// Gone reports whether a load balancer is gone: 404, or the API keeps the row as Deleted. A delete Incus refused puts
// the status back to Available (or NotFound), with nothing pending and the reason set.
func Gone(status int, lb *api.LoadBalancerQueryResult) (bool, error) {
	if status == http.StatusNotFound {
		return true, nil
	}
	if lb == nil {
		return false, nil
	}
	if hasStatus(lb, api.LoadBalancerStatusDeleted) {
		return true, nil
	}
	refused := !waits(lb) && lb.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the load balancer was not deleted: %s", *lb.RequestFailureReason)
	}
	return false, nil
}

func sameListeners(a, b []Listener) bool {
	key := func(l Listener) string { return fmt.Sprintf("%s %d %d", l.Protocol, l.Port, l.TargetPort) }
	var ka, kb []string
	for _, l := range a {
		ka = append(ka, key(l))
	}
	for _, l := range b {
		kb = append(kb, key(l))
	}
	return sameStrings(ka, kb)
}

func sameStrings(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func number(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}
