package tui

// fuzzyMatch performs a simple subsequence fuzzy match of query against text
// (both assumed lowercased). It returns the matched rune indices in text, a
// score favoring contiguous and early matches, and whether all query runes
// were found in order.
func fuzzyMatch(text, query string) (indices []int, score int, ok bool) {
	if query == "" {
		return nil, 0, true
	}
	tr := []rune(text)
	qr := []rune(query)

	qi := 0
	prevMatch := -2
	for ti := 0; ti < len(tr) && qi < len(qr); ti++ {
		if tr[ti] != qr[qi] {
			continue
		}
		indices = append(indices, ti)
		// Contiguity bonus: matching right after the previous match scores well.
		if ti == prevMatch+1 {
			score += 5
		}
		// Earliness bonus: earlier matches are slightly preferred.
		score += 10 - min(ti, 10)
		prevMatch = ti
		qi++
	}
	if qi != len(qr) {
		return nil, 0, false
	}
	// Shorter haystacks that fully match rank higher.
	score += max(0, 20-len(tr)/4)
	return indices, score, true
}
