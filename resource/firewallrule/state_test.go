package firewallrule

import (
	"strings"
	"testing"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

func ptr[T any](v T) *T { return &v }

func aRule(id string, status api.FirewallRuleStatus) api.FirewallRuleQueryResult {
	return api.FirewallRuleQueryResult{Id: ptr(id), Status: &status}
}

func TestFind_PicksTheRuleById(t *testing.T) {
	rules := []api.FirewallRuleQueryResult{aRule("a", api.FirewallRuleStatusEnabled), aRule("b", api.FirewallRuleStatusEnabled)}

	if found := Find(rules, "b"); found == nil || *found.Id != "b" {
		t.Fatalf("found = %+v", found)
	}
	if Find(rules, "c") != nil {
		t.Fatal("a rule not in the list must not be found")
	}
}

func TestCreated_WhileCreating_KeepsWaiting_ThenDone(t *testing.T) {
	if done, err := Created(aRule("a", api.FirewallRuleStatusCreating)); done || err != nil {
		t.Fatalf("creating: done = %v, err = %v", done, err)
	}
	for _, status := range []api.FirewallRuleStatus{api.FirewallRuleStatusEnabled, api.FirewallRuleStatusDisabled,
		api.FirewallRuleStatusLogged, api.FirewallRuleStatusUnknown} {
		if done, err := Created(aRule("a", status)); !done || err != nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

func TestCreated_AStatusTheProviderDoesNotKnow_KeepsWaiting(t *testing.T) {
	if done, err := Created(aRule("a", api.FirewallRuleStatus("Rebuilding"))); done || err != nil {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_RequestGivenUp_FailsWithTheReason(t *testing.T) {
	rule := aRule("a", api.FirewallRuleStatusFailed)
	rule.RequestFailureReason = ptr("ACL refused")

	done, err := Created(rule)
	if done || err == nil || !strings.Contains(err.Error(), "ACL refused") {
		t.Fatalf("done = %v, err = %v", done, err)
	}
}

func TestCreated_FailedNotFoundOrDeleted_FailsWithoutAReason(t *testing.T) {
	for _, status := range []api.FirewallRuleStatus{api.FirewallRuleStatusFailed, api.FirewallRuleStatusNotFound, api.FirewallRuleStatusDeleted} {
		if done, err := Created(aRule("a", status)); done || err == nil {
			t.Fatalf("%s: done = %v, err = %v", status, done, err)
		}
	}
}

func TestGone_LeavingTheList_IsGone_ARefusedDelete_Fails(t *testing.T) {
	if done, err := Gone(nil); !done || err != nil {
		t.Fatalf("not listed: done = %v, err = %v", done, err)
	}
	deleted := aRule("a", api.FirewallRuleStatusDeleted)
	if done, err := Gone(&deleted); !done || err != nil {
		t.Fatalf("deleted: done = %v, err = %v", done, err)
	}
	deleting := aRule("a", api.FirewallRuleStatusDeleting)
	if done, err := Gone(&deleting); done || err != nil {
		t.Fatalf("deleting: done = %v, err = %v", done, err)
	}
	refused := aRule("a", api.FirewallRuleStatusEnabled)
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

func TestDeletable_AtRestOrDeleting_Goes(t *testing.T) {
	for _, status := range []api.FirewallRuleStatus{api.FirewallRuleStatusEnabled, api.FirewallRuleStatusDeleting, api.FirewallRuleStatusFailed} {
		rule := aRule("r", status)
		if !Deletable(&rule) {
			t.Fatalf("status %s", status)
		}
	}
	if !Deletable(nil) {
		t.Fatal("a rule no longer listed needs no wait")
	}
}

func TestDeletable_WhileCreating_Waits(t *testing.T) {
	rule := aRule("r", api.FirewallRuleStatusCreating)
	if Deletable(&rule) {
		t.Fatal("the API refuses to delete a rule whose create is pending")
	}
}
