package tui

import (
	"reflect"
	"testing"
)

// TestFuzzyMatch covers the subsequence-matching contract: whether a query
// matches, and which rune positions in the haystack were consumed.
func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		query   string
		wantOK  bool
		wantIdx []int
	}{
		{"empty query matches all with nil indices", "anything", "", true, nil},
		{"empty query matches empty text", "", "", true, nil},
		{"non-match", "main", "zzz", false, nil},
		{"partial subsequence is not enough", "pay", "payment", false, nil},
		{"exact match", "pay", "pay", true, []int{0, 1, 2}},
		{"scattered subsequence", "feature-payments", "fp", true, []int{0, 8}},
		{"indices are the matched rune positions", "feature-payments", "pay", true, []int{8, 9, 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, _, ok := fuzzyMatch(tt.text, tt.query)
			if ok != tt.wantOK {
				t.Fatalf("fuzzyMatch(%q, %q) ok=%v, want %v", tt.text, tt.query, ok, tt.wantOK)
			}
			if !ok {
				return // indices are meaningless for a non-match
			}
			if !reflect.DeepEqual(idx, tt.wantIdx) {
				t.Fatalf("fuzzyMatch(%q, %q) indices=%v, want %v", tt.text, tt.query, idx, tt.wantIdx)
			}
		})
	}
}

// TestFuzzyMatchRanking asserts the relative-score properties recompute relies
// on when sorting filtered rows (higher score ranks first).
func TestFuzzyMatchRanking(t *testing.T) {
	score := func(t *testing.T, text, query string) int {
		t.Helper()
		_, s, ok := fuzzyMatch(text, query)
		if !ok {
			t.Fatalf("fuzzyMatch(%q, %q) unexpectedly failed", text, query)
		}
		return s
	}
	tests := []struct {
		name   string
		query  string
		better string // haystack expected to score strictly higher
		worse  string
	}{
		// A short exact hit must beat the same query scattered/embedded in a
		// longer branch name (this is what puts "pay" above "feature-payments").
		{"contiguous match beats scattered", "pay", "pay", "feature-payments"},
		// Same-length haystacks: the earlier occurrence wins.
		{"earlier match beats later", "pay", "pay-one", "one-pay"},
		// Identical contiguous match at position 0: the shorter haystack wins.
		{"shorter haystack bonus", "pay", "pay-1", "pay-something-longer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, w := score(t, tt.better, tt.query), score(t, tt.worse, tt.query)
			if b <= w {
				t.Fatalf("score(%q)=%d should beat score(%q)=%d for query %q",
					tt.better, b, tt.worse, w, tt.query)
			}
		})
	}
}
