// Package fold sorts strings as people read them, for the processes and the CLI alike.
package fold

import (
	"bytes"
	"slices"
	"sync"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// collators are pooled: a collate.Collator is not safe for concurrent use.
// The root order is the same for fr and en (CLDR has no fr-FR rules), so the account's locale is not needed.
var collators = sync.Pool{New: func() any { return collate.New(language.Und, collate.IgnoreCase) }}

// Sort sorts items by first, then by key whatever its case and with accented letters next to their base letter
// (Été between abricot and zoo), as the collation of Postgres did. first may be nil; the sort is stable.
// Each key is collated once, not on every comparison: 50,000 paths sort in tens of ms, not hundreds.
func Sort[T any](items []T, key func(T) string, first func(a, b T) int) {
	c := collators.Get().(*collate.Collator)
	defer collators.Put(c)
	// Indexes sorted, not the items: a stable sort moves big items in O(n log² n), 150 ms for 50,000 notes.
	type keyed struct {
		index int
		key   []byte
	}
	var buf collate.Buffer
	all := make([]keyed, len(items))
	for i, item := range items {
		all[i] = keyed{i, c.KeyFromString(&buf, key(item))}
	}
	slices.SortFunc(all, func(a, b keyed) int {
		if first != nil {
			if order := first(items[a.index], items[b.index]); order != 0 {
				return order
			}
		}
		if order := bytes.Compare(a.key, b.key); order != 0 {
			return order
		}
		return a.index - b.index // stable
	})
	sorted := make([]T, len(items))
	for i, k := range all {
		sorted[i] = items[k.index]
	}
	copy(items, sorted)
}
