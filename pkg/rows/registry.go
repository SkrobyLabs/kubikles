package rows

import (
	"fmt"
	"sort"
	"sync"

	"kubikles/pkg/store"
)

var (
	regMu    sync.RWMutex
	registry = map[string]*Spec{}
)

// Register validates and adds a spec. A type is migrated exactly when its spec is registered.
func Register(s *Spec) error {
	if err := validate(s); err != nil {
		return err
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := registry[s.Type]; dup {
		return fmt.Errorf("rows: spec %q registered twice", s.Type)
	}
	registry[s.Type] = s
	return nil
}

// MustRegister is Register for package initialisation.
func MustRegister(s *Spec) {
	if err := Register(s); err != nil {
		panic(err)
	}
}

// Unregister removes a spec (the per-kind revert switch and tests).
func Unregister(typ string) {
	regMu.Lock()
	defer regMu.Unlock()
	delete(registry, typ)
}

// Lookup returns the spec for a type.
func Lookup(typ string) (*Spec, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	s, ok := registry[typ]
	return s, ok
}

// Types lists the registered type strings, sorted.
func Types() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func validate(s *Spec) error {
	if s == nil || s.Type == "" {
		return fmt.Errorf("rows: spec without a type")
	}
	if len(s.Columns) == 0 {
		return fmt.Errorf("rows: spec %q has no columns", s.Type)
	}
	keys := map[string]bool{}
	for _, c := range s.Columns {
		if c.Key == "" {
			return fmt.Errorf("rows: spec %q has a column without a key", s.Type)
		}
		if keys[c.Key] {
			return fmt.Errorf("rows: spec %q repeats column %q", s.Type, c.Key)
		}
		keys[c.Key] = true
		if c.Cell == nil {
			return fmt.Errorf("rows: spec %q column %q has no Cell", s.Type, c.Key)
		}
		switch c.Kind {
		case KindString, KindNumber, KindTime, KindBool:
		default:
			return fmt.Errorf("rows: spec %q column %q has kind %q", s.Type, c.Key, c.Kind)
		}
		switch c.FilterType {
		case FilterSelect, FilterRegex, FilterNumeric, FilterNone:
		default:
			return fmt.Errorf("rows: spec %q column %q has filter type %q", s.Type, c.Key, c.FilterType)
		}
		if c.CellType == "" {
			return fmt.Errorf("rows: spec %q column %q has no CellType", s.Type, c.Key)
		}
		if len(c.Predefined) > 0 && c.FilterType != FilterSelect {
			return fmt.Errorf("rows: spec %q column %q has predefined values but is not a select filter", s.Type, c.Key)
		}
	}
	names := map[string]bool{}
	for _, f := range s.Search {
		if f.Name == "" || f.Extract == nil {
			return fmt.Errorf("rows: spec %q has an incomplete search field", s.Type)
		}
		if names[f.Name] {
			return fmt.Errorf("rows: spec %q repeats search field %q", s.Type, f.Name)
		}
		names[f.Name] = true
	}
	if _, ok := names["name"]; !ok && len(s.Search) > 0 {
		return fmt.Errorf("rows: spec %q has search fields but no name field", s.Type)
	}
	for _, d := range s.Derived {
		if d.From != "pods" {
			return fmt.Errorf("rows: spec %q derives from %q, only pods is supported", s.Type, d.From)
		}
		if d.Selector == nil || len(d.Columns) == 0 {
			return fmt.Errorf("rows: spec %q has an incomplete derived source", s.Type)
		}
		for _, col := range d.Columns {
			if !keys[col] {
				return fmt.Errorf("rows: spec %q derives unknown column %q", s.Type, col)
			}
		}
	}
	for _, o := range s.Overlays {
		if o.Name == "" || len(o.Columns) == 0 || o.Key == nil {
			return fmt.Errorf("rows: spec %q has an empty overlay", s.Type)
		}
		for _, col := range o.Columns {
			if !keys[col] {
				return fmt.Errorf("rows: spec %q overlay %q targets unknown column %q", s.Type, o.Name, col)
			}
		}
	}
	presets := map[string]bool{}
	for _, p := range s.Presets {
		if p.Name == "" || p.Hide == nil || presets[p.Name] {
			return fmt.Errorf("rows: spec %q has an invalid or repeated preset %q", s.Type, p.Name)
		}
		presets[p.Name] = true
	}
	switch s.Needs {
	case store.ModeFull, store.ModeTable, store.ModeMetadata:
	default:
		return fmt.Errorf("rows: spec %q has unknown Needs", s.Type)
	}
	return nil
}
