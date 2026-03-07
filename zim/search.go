package zim

import (
	"iter"
	"strings"
	"unicode"
)

// EntriesByTitlePrefix returns an iterator over entries in namespace ns whose
// title starts with prefix (case-sensitive). Binary search locates the lower
// bound in the title pointer list — O(log N) to find the start, O(k) to
// iterate k results.
//
// An empty prefix iterates all entries in the namespace in title order.
func (a *Archive) EntriesByTitlePrefix(ns byte, prefix string) iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		// Binary search for lower bound: first index where
		// compareTitleKey(entry) >= (ns, prefix).
		lo, hi := uint32(0), a.hdr.EntryCount
		for lo < hi {
			mid := lo + (hi-lo)/2
			e, err := a.entryByTitleIndex(mid)
			if err != nil {
				return
			}
			if compareTitleKey(e.Namespace(), e.Title(), ns, prefix) < 0 {
				lo = mid + 1
			} else {
				hi = mid
			}
		}

		// Iterate forward while namespace matches and title has the prefix.
		for i := lo; i < a.hdr.EntryCount; i++ {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if e.Namespace() != ns {
				return
			}
			if !strings.HasPrefix(e.Title(), prefix) {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// EntriesByTitlePrefixFold is like EntriesByTitlePrefix but case-insensitive
// using Unicode simple case folding. Because the title list is sorted
// case-sensitively, this requires a full linear scan — O(N). Prefer
// EntriesByTitlePrefix when case sensitivity is acceptable.
func (a *Archive) EntriesByTitlePrefixFold(ns byte, prefix string) iter.Seq[Entry] {
	folded := strings.ToLower(prefix)
	return func(yield func(Entry) bool) {
		for i := uint32(0); i < a.hdr.EntryCount; i++ {
			e, err := a.entryByTitleIndex(i)
			if err != nil {
				return
			}
			if e.Namespace() != ns {
				continue
			}
			if hasPrefixFold(e.Title(), folded) {
				if !yield(e) {
					return
				}
			}
		}
	}
}

// hasPrefixFold reports whether s starts with prefix under Unicode case folding.
func hasPrefixFold(s, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix) ||
		strings.HasPrefix(strings.Map(unicode.ToLower, s), prefix)
}

// HasFulltextIndex reports whether the archive contains an embedded full-text
// Xapian index (X/fulltext/xapian). Such indexes are created by libzim 7+ and
// enable full-text search via external Xapian tools.
func (a *Archive) HasFulltextIndex() bool {
	_, err := a.EntryByPath("X/fulltext/xapian")
	return err == nil
}

// HasTitleIndex reports whether the archive contains an embedded title
// suggestion Xapian index (X/title/xapian).
func (a *Archive) HasTitleIndex() bool {
	_, err := a.EntryByPath("X/title/xapian")
	return err == nil
}
