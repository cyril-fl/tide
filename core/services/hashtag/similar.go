package hashtag

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Similar groups the tags that look like one tag written apart: the same once case and accents are folded and a
// trailing s or x dropped, or a few typos away (1 edit from 5 runes, 2 from 8, none shorter: go and js stay apart).
// A group gathers every tag linked to another by that rule, in the order of tags; a tag close to none is left out.
// Each tag is compared to those of a length close enough only, so 1 000 tags stay fast.
func Similar[T any](tags []T, value func(T) string) [][]T {
	keys := make([][]rune, len(tags))
	order := make([]int, len(tags))
	for i, tag := range tags {
		keys[i], order[i] = fold(value(tag)), i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(len(keys[a]), len(keys[b])) })

	parent := make([]int, len(tags))
	for i := range parent {
		parent[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	for x, a := range order {
		for _, b := range order[x+1:] {
			allowed := typos(len(keys[a]))
			if len(keys[b])-len(keys[a]) > allowed {
				break // sorted by length: every later key is longer still
			}
			if distance(keys[a], keys[b], allowed) <= allowed {
				parent[root(b)] = root(a)
			}
		}
	}

	groups := [][]T{} // an empty list, not null, in JSON
	at := map[int]int{}
	for i, tag := range tags {
		r := root(i)
		if _, ok := at[r]; !ok {
			at[r] = len(groups)
			groups = append(groups, nil)
		}
		groups[at[r]] = append(groups[at[r]], tag)
	}
	return slices.DeleteFunc(groups, func(group []T) bool { return len(group) < 2 })
}

// fold lowers the tag, strips its accents and drops a plural s or x: DéveloppementS and developpement are one key.
func fold(tag string) []rune {
	var key []rune
	for _, r := range norm.NFD.String(strings.ToLower(tag)) {
		if !unicode.Is(unicode.Mn, r) {
			key = append(key, r)
		}
	}
	if n := len(key); n > 3 && (key[n-1] == 's' || key[n-1] == 'x') {
		key = key[:n-1]
	}
	return key
}

// typos is the edits allowed between two keys, the shorter of length n: a typo in a short tag makes another word.
func typos(n int) int {
	switch {
	case n < 5:
		return 0
	case n < 8:
		return 1
	default:
		return 2
	}
}

// distance is the Levenshtein distance of a and b, or max+1 once a row passes max.
func distance(a, b []rune, max int) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		prev, least := row[0], i
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			prev, row[j] = row[j], min(row[j]+1, row[j-1]+1, prev+cost)
			least = min(least, row[j])
		}
		if least > max {
			return max + 1
		}
	}
	return row[len(b)]
}
