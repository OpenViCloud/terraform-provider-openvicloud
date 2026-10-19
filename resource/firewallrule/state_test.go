package firewallrule

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aRule(id string, existence api.CurrentExistence, transition *api.FirewallRuleTransition) api.FirewallRuleQueryResult {
	return api.FirewallRuleQueryResult{Id: ptr(id), CurrentExistence: &existence, Transition: transition,
		DesiredExistence: ptr(api.DesiredExistenceExists)}
}

func TestFind_PicksTheRuleById(t *testing.T) {
	rules := []api.FirewallRuleQueryResult{aRule("a", api.CurrentExistenceExists, nil), aRule("b", api.CurrentExistenceExists, nil)}

	if found := Find(rules, "b"); found == nil || *found.Id != "b" {
		t.Fatalf("found = %+v", found)
	}
	if Find(rules, "c") != nil {
		t.Fatal("a rule not in the list must not be found")
	}
}

func TestCreated_WhileCreating_KeepsWaiting_ThenDone(t *testing.T) {
	if done, err := Created(aRule("a", api.CurrentExistenceNotCreated, ptr(api.FirewallRuleTransitionCreating))); done || err != nil {
		t.Fatalf("creating: done = %v, err = %v", done, err)
	}
	if done, err := Created(aRule("a", api.CurrentExistenceExists, nil)); !done || err != nil {
		t.Fatalf("exists: done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	rule := aRule("a", api.CurrentExistenceNotCreated, nil)
	rule.RequestFailureReason = ptr("ACL refused")

	done, err := Created(rule)
	if done || err == nil || !strings.Contains(err.Error(), "ACL refused") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestGone_LeavingTheList_IsGone_ARefusedDelete_Fails(t *testing.T) {
	if done, err := Gone(nil); !done || err != nil {
		t.Fatalf("not listed: done = %v, err = %v", done, err)
	}
	deleting := aRule("a", api.CurrentExistenceExists, ptr(api.FirewallRuleTransitionDeleting))
	deleting.DesiredExistence = ptr(api.DesiredExistenceDeleted)
	if done, err := Gone(&deleting); done || err != nil {
		t.Fatalf("deleting: done = %v, err = %v", done, err)
	}
	refused := aRule("a", api.CurrentExistenceExists, nil)
	refused.RequestFailureReason = ptr("ACL locked")
	if done, err := Gone(&refused); done || err == nil || !strings.Contains(err.Error(), "ACL locked") {
		t.Fatalf("refused: done = %v, err = %v", done, err)
	}
}

func TestImportId_IsNetworkSlashRule(t *testing.T) {
	network, rule, ok := SplitImportId("n-1/r-2")
	if !ok || network != "n-1" || rule != "r-2" {
		t.Fatalf("network = %q, rule = %q, ok = %v", network, rule, ok)
	}
	for _, bad := range []string{"r-2", "/r-2", "n-1/", "a/b/c"} {
		if _, _, ok := SplitImportId(bad); ok {
			t.Fatalf("%q must be refused", bad)
		}
	}
}
