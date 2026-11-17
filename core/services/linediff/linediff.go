// Package linediff lists the lines that differ between two texts, for the previews of the bulk edits.
package linediff

import "strings"

// maxCells bounds the table of the lines left once the common head and tail are cut: past it, they are all listed.
const maxCells = 4_000_000

// Lines returns the lines of old gone as "- line" and those of next new as "+ line", in text order; the lines both
// keep are left out. Equal texts give nil.
func Lines(old, next string) []string {
	a, b := strings.Split(old, "\n"), strings.Split(next, "\n")
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}
	tail := 0
	for tail < len(a)-head && tail < len(b)-head && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	a, b = a[head:len(a)-tail], b[head:len(b)-tail]

	var out []string
	if (len(a)+1)*(len(b)+1) > maxCells {
		// TODO: no alignment on a huge change, every line of the region is listed; align it if it is ever needed.
		for _, l := range a {
			out = append(out, "- "+l)
		}
		for _, l := range b {
			out = append(out, "+ "+l)
		}
		return out
	}

	// lcs[i][j]: the longest common run of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	return out
}
