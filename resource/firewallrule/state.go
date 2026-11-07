// Package firewallrule is the openvicloud_firewall_rule resource: one line in the ACL of a network. A rule cannot
// change in place; the API has no GET for one rule, only the list of its network.
package firewallrule

import (
	"fmt"
	"strings"

	"github.com/OpenViCloud/terraform-provider-openvicloud/api"
)

// Find returns the rule with id in the list of its network, or nil: a deleted rule leaves the list.
func Find(rules []api.FirewallRuleQueryResult, id string) *api.FirewallRuleQueryResult {
	for i := range rules {
		if rules[i].Id != nil && *rules[i].Id == id {
			return &rules[i]
		}
	}
	return nil
}

// Created reports whether a create is done: Incus holds the rule (enabled, disabled or logged), or the API has not read
// it back yet (Unknown with no failure reason, which a rule has only after a create that succeeded). A reason fails the
// create whatever the status; Failed, NotFound and Deleted fail it; Creating and a status word the provider does not
// know keep it waiting.
func Created(rule api.FirewallRuleQueryResult) (bool, error) {
	if rule.Status == nil {
		return false, nil
	}
	if rule.RequestFailureReason != nil {
		return false, fmt.Errorf("the firewall rule was not created, it is %s: %s", *rule.Status, *rule.RequestFailureReason)
	}
	switch *rule.Status {
	case api.FirewallRuleStatusEnabled, api.FirewallRuleStatusDisabled, api.FirewallRuleStatusLogged, api.FirewallRuleStatusUnknown:
		return true, nil
	case api.FirewallRuleStatusFailed, api.FirewallRuleStatusNotFound:
		return false, fmt.Errorf("the firewall rule was not created, it is %s", *rule.Status)
	case api.FirewallRuleStatusDeleted:
		return false, fmt.Errorf("the firewall rule was deleted before its create finished")
	}
	return false, nil
}

// Gone reports whether a rule is gone: no longer in the list, or Deleted. A delete Incus refused returns the rule to
// an Incus word (Unknown when none is known), with the reason set; Deleting, and Creating, are waited out.
func Gone(rule *api.FirewallRuleQueryResult) (bool, error) {
	if rule == nil {
		return true, nil
	}
	if rule.Status == nil {
		return false, nil
	}
	if *rule.Status == api.FirewallRuleStatusDeleted {
		return true, nil
	}
	refused := *rule.Status != api.FirewallRuleStatusDeleting && *rule.Status != api.FirewallRuleStatusCreating &&
		rule.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the firewall rule was not deleted, it is %s: %s", *rule.Status, *rule.RequestFailureReason)
	}
	return false, nil
}

// SplitImportId reads the import id <network id>/<rule id>.
func SplitImportId(id string) (network string, rule string, ok bool) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Deletable reports whether the API takes a delete now: it refuses one while the create is pending. A pending delete is
// repeated; a rule no longer listed needs no wait.
func Deletable(rule *api.FirewallRuleQueryResult) bool {
	return rule == nil || rule.Status == nil || *rule.Status != api.FirewallRuleStatusCreating
}
