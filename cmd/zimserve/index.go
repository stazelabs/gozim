package main

import "net/http"

func (lib *library) writeIndexPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'")

	entries := make([]indexEntry, len(lib.slugs))
	for i, slug := range lib.slugs {
		e := lib.archives[slug]
		entries[i] = indexEntry{
			Slug:        slug,
			Title:       e.title,
			Description: e.description,
			Filename:    e.filename,
			Language:    e.language,
			Creator:     e.creator,
			Flavour:     e.flavour,
			Date:        e.date,
			EntryCount:  int(e.archive.EntryCount()),
		}
	}
	renderWith(w, tmplIndex, indexData{
		SingleZIM: len(lib.slugs) == 1,
		NoInfo:    lib.noInfo,
		Entries:   entries,
	})
}
