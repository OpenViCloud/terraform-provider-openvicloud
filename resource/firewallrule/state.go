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

// Created reports whether a create is done: the rule exists in the ACL and nothing is pending.
func Created(rule api.FirewallRuleQueryResult) (bool, error) {
	if rule.Transition != nil {
		return false, nil
	}
	if rule.RequestFailureReason != nil {
		return false, fmt.Errorf("the firewall rule was not created: %s", *rule.RequestFailureReason)
	}
	return rule.CurrentExistence != nil && *rule.CurrentExistence == api.CurrentExistenceExists, nil
}

// Gone reports whether a rule is gone: no longer in the list. A delete Incus refused puts the desired existence back
// to Exists, with nothing pending and the reason set.
func Gone(rule *api.FirewallRuleQueryResult) (bool, error) {
	if rule == nil {
		return true, nil
	}
	refused := rule.Transition == nil && rule.DesiredExistence != nil && *rule.DesiredExistence == api.DesiredExistenceExists &&
		rule.RequestFailureReason != nil
	if refused {
		return false, fmt.Errorf("the firewall rule was not deleted: %s", *rule.RequestFailureReason)
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
