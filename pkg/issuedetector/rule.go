package issuedetector

import "context"

// Rule is the interface that all issue detection rules must implement.
type Rule interface {
	ID() string
	Name() string
	Description() string
	Severity() Severity
	Category() Category
	RequiredResources() []string // e.g. "pods", "services", "ingresses"
	Evaluate(ctx context.Context, cache *ResourceCache) ([]Finding, error)
}

// baseRule provides common fields for built-in rules.
type baseRule struct {
	id          string
	name        string
	description string
	severity    Severity
	category    Category
	requires    []string
}

func (r *baseRule) ID() string                  { return r.id }
func (r *baseRule) Name() string                { return r.name }
func (r *baseRule) Description() string         { return r.description }
func (r *baseRule) Severity() Severity          { return r.severity }
func (r *baseRule) Category() Category          { return r.category }
func (r *baseRule) RequiredResources() []string { return r.requires }

// makeFinding is a helper to create a Finding from a rule.
func makeFinding(r Rule, ref ResourceRef, desc, fix string, details map[string]string) Finding {
	return Finding{
		RuleID:       r.ID(),
		RuleName:     r.Name(),
		Severity:     r.Severity(),
		Category:     r.Category(),
		Resource:     ref,
		Description:  desc,
		SuggestedFix: fix,
		Details:      details,
	}
}
