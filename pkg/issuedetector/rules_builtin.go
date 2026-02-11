package issuedetector

// registerBuiltinRules returns all built-in rules.
func registerBuiltinRules() []Rule {
	var rules []Rule
	rules = append(rules, networkingRules()...)
	rules = append(rules, workloadRules()...)
	rules = append(rules, storageRules()...)
	rules = append(rules, securityRules()...)
	rules = append(rules, configRules()...)
	rules = append(rules, deprecationRules()...)
	rules = append(rules, nodeRules()...)
	rules = append(rules, costRules()...)
	rules = append(rules, certRules()...)
	rules = append(rules, rbacRules()...)
	rules = append(rules, schedulingRules()...)
	return rules
}
