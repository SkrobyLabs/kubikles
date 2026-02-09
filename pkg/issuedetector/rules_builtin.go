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
	return rules
}
