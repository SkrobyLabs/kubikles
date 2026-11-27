package rows

import (
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// newCollator builds the name collator for a client locale. An empty or unparsable locale
// uses the root collation, which matches `new Intl.Collator()` in an English environment.
//
// Differences from ICU are limited to locale tailorings the CLDR tables in golang.org/x/text
// treat differently from the browser's ICU version; the golden files pin the English behaviour.
func newCollator(locale string) *collate.Collator {
	tag := language.Und
	if locale != "" {
		if t, err := language.Parse(locale); err == nil {
			tag = t
		}
	}
	return collate.New(tag)
}

// sortPlan is the compiled sort of a view: sortItems from resourceListPipeline.ts. The primary
// key is the sort column's cell compared with the JavaScript relational operators; ties fall
// back to newest creationTimestamp first (unless sorting by age), then the name through the
// collator (unless sorting by name). Namespace and identity close the order so it is total.
type sortPlan struct {
	col     *Column // nil: no sort configured (legacy leaves the data as it arrived)
	colIdx  int     // index of col in the projected cells, -1 when not projected
	asc     bool
	tieAge  bool
	tieName bool
	coll    interface{ CompareString(a, b string) int }
}

func newSortPlan(spec *Spec, key, dir string, coll interface{ CompareString(a, b string) int }) sortPlan {
	p := sortPlan{colIdx: -1, asc: dir == "asc", tieAge: key != "age", tieName: key != "name", coll: coll}
	if key != "" {
		p.col = spec.Column(key)
	}
	return p
}

// compare orders two rows; the result is never 0 for distinct rows.
func (p *sortPlan) compare(a, b *viewRow) int {
	if p.col != nil {
		if c := compareCells(a.sortKey, b.sortKey); c != cmpEqual {
			if p.asc {
				return int(c)
			}
			return -int(c)
		}
		// Tie-breakers are always ascending: "ascending age" means newest first.
		if p.tieAge && a.ts != "" && b.ts != "" && a.ts != b.ts {
			if a.ts > b.ts {
				return -1
			}
			return 1
		}
		if p.tieName {
			if c := p.coll.CompareString(a.name, b.name); c != 0 {
				return c
			}
		}
	}
	if c := CompareUTF16(a.ns, b.ns); c != 0 {
		return c
	}
	if p.col == nil {
		if c := CompareUTF16(a.name, b.name); c != 0 {
			return c
		}
	}
	return CompareUTF16(a.id, b.id)
}
