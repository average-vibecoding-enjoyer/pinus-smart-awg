package smart

// DNS follows explicit domain priorities. Application/CIDR attribution does
// not exist for all Windows DNS queries (the system resolver can own them),
// so unclassified DNS stays inside AWG rather than guessing the application.
func policyDNSRules(settings RoutingSettings, vpnServer, vpnStrategy string) []map[string]any {
	var rules []map[string]any
	add := func(rule map[string]any, vpn bool) {
		if _, domains := rule["domain"]; !domains {
			return
		}
		copyRule := map[string]any{"action": "route", "server": vpnServer, "strategy": vpnStrategy}
		copyRule["domain"] = rule["domain"]
		if suffixes, ok := rule["domain_suffix"]; ok {
			copyRule["domain_suffix"] = suffixes
		}
		if !vpn {
			copyRule["server"] = "direct-dns"
			copyRule["strategy"] = "prefer_ipv4"
		}
		rules = append(rules, copyRule)
	}
	for _, rule := range settings.CustomRules {
		if rule.Enabled && rule.Kind == RuleDomain {
			add(customRuleObject(rule), rule.Target == TargetVPN)
		}
	}
	for _, rule := range regionalRuleObjects(settings) {
		add(rule, rule["outbound"] == "awg-out")
	}
	return rules
}
