package rows

import (
	"math"
	"sort"
	"time"
)

// Namespace query planning: a port of frontend/src/utils/namespaceQuery.ts. The plan says
// whether a selection of namespaces is served by one cluster-wide shard or by one shard per
// namespace.

const (
	observationTTL = 5 * time.Minute
	mib            = 1024 * 1024
	// QueryConcurrency is NAMESPACE_QUERY_CONCURRENCY.
	QueryConcurrency = 4
)

// NamespaceSelection normalises a namespace selection: nil for none, ["*"] when everything (or an
// empty name) is selected, otherwise the unique names in UTF-16 order.
func NamespaceSelection(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "*" || v == "" {
			return []string{"*"}
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return CompareUTF16(out[i], out[j]) < 0 })
	return out
}

// Observation is what a previous cluster-wide query cost.
type Observation struct {
	Count      int
	Bytes      int64
	DurationMs float64
	ObservedAt time.Time
}

// PlanOptions are the optional inputs of ChooseNamespaceScope.
type PlanOptions struct {
	// BroadDenied is set once a cluster-wide request was forbidden for this user.
	BroadDenied bool
	// ReuseBroad prefers an already-warm cluster-wide result even for one namespace.
	ReuseBroad bool
	// ScopedDurationMs is how long one per-namespace request took; nil means unknown.
	ScopedDurationMs *float64
	// Now overrides the clock.
	Now time.Time
}

// ChooseNamespaceScope returns the namespaces to query: [""] for one cluster-wide query, the
// selected names for per-namespace queries, nil for an empty selection.
func ChooseNamespaceScope(selected, available []string, obs *Observation, o PlanOptions) []string {
	if len(selected) == 0 {
		return nil
	}
	for _, s := range selected {
		if s == "*" {
			return []string{""}
		}
	}
	if o.BroadDenied {
		return selected
	}
	known := make(map[string]struct{}, len(available))
	for _, a := range available {
		if a != "" && a != "*" {
			known[a] = struct{}{}
		}
	}
	coverage := 0.0
	if len(known) > 0 {
		in := 0
		for _, s := range selected {
			if _, ok := known[s]; ok {
				in++
			}
		}
		coverage = float64(in) / float64(len(known))
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	if obs != nil && now.Sub(obs.ObservedAt) < observationTTL {
		// Object count alone misses large secrets and configmaps; payload size and latency count too.
		small := obs.Count <= 500 && obs.Bytes <= 5*mib && obs.DurationMs <= 2000
		if small && (len(selected) >= 2 || o.ReuseBroad) {
			return []string{""}
		}
		scopedEstimate := 2000.0
		if o.ScopedDurationMs != nil {
			scopedEstimate = math.Ceil(float64(len(selected))/QueryConcurrency) * *o.ScopedDurationMs
		}
		if len(selected) >= 4 && coverage >= 0.8 && obs.Count <= 5000 &&
			obs.Bytes <= 20*mib && obs.DurationMs <= scopedEstimate {
			return []string{""}
		}
		return selected
	}
	// No cluster-wide counts yet: do not infer density from a few possibly empty namespaces.
	if len(selected) >= 4 && coverage >= 0.5 {
		return []string{""}
	}
	return selected
}
