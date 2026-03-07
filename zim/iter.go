package zim

import "iter"

// Entries returns an iterator over all entries sorted by path (URL order).
func (a *Archive) Entries() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for i := uint32(0); i < a.hdr.EntryCount; i++ {
			e, err := a.EntryByIndex(i)
			if err != nil {
				return
			}
			if !yield(e) {
				return
			}
		}
	}
}

// EntriesByNamespace returns an iterator over entries in a specific namespace.
func (a *Archive) EntriesByNamespace(ns byte) iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		for i := uint32(0); i < a.hdr.EntryCount; i++ {
			e, err := a.EntryByIndex(i)
			if err != nil {
				return
			}
			if e.Namespace() != ns {
				continue
			}
			if !yield(e) {
				return
			}
		}
	}
}
