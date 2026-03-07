package main

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// handleInfo serves GET /{slug}/-/info — diagnostics/metadata page for a ZIM file.
func (lib *library) handleInfo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	a := ze.archive

	// Gather namespace counts by probing known namespaces via binary search.
	// This avoids iterating all entries — O(k log N) where k is the number of
	// known namespaces, instead of O(N).
	type nsCount struct {
		NS    byte
		Count int
	}
	var nsCounts []nsCount
	for _, ns := range []byte{'-', 'A', 'C', 'I', 'M', 'V', 'W', 'X'} {
		if c := a.EntryCountByNamespace(ns); c > 0 {
			nsCounts = append(nsCounts, nsCount{NS: ns, Count: c})
		}
	}

	// Count MIME types by iterating C namespace
	mimeCounts := make(map[string]int)
	redirectCount := 0
	for e := range a.EntriesByTitlePrefix('C', "") {
		if e.IsRedirect() {
			redirectCount++
		} else {
			mime := e.MIMEType()
			if mime == "" {
				mime = "(unknown)"
			}
			mimeCounts[mime]++
		}
	}
	type mimeCount struct {
		MIME  string
		Count int
	}
	var mimeList []mimeCount
	for m, c := range mimeCounts {
		mimeList = append(mimeList, mimeCount{m, c})
	}
	sort.Slice(mimeList, func(i, j int) bool { return mimeList[i].Count > mimeList[j].Count })

	// Metadata keys to try
	metaKeys := []string{
		"Title", "Creator", "Publisher", "Date", "Description",
		"LongDescription", "Language", "License", "Tags", "Relation",
		"Flavour", "Source", "Counter", "Scraper",
	}
	type metaEntry struct {
		Key   string
		Value string
	}
	var metadata []metaEntry
	for _, key := range metaKeys {
		val, err := a.Metadata(key)
		if err == nil && val != "" {
			metadata = append(metadata, metaEntry{key, val})
		}
	}

	uuid := a.UUID()
	uuidStr := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Info — %s</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 900px; margin: 40px auto; padding: 0 20px; }
h1 { font-size: 1.4em; border-bottom: 1px solid #ddd; padding-bottom: 10px; }
h2 { font-size: 1.15em; margin-top: 28px; color: #333; }
table { border-collapse: collapse; width: 100%%; margin-bottom: 16px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #eee; }
th { width: 200px; color: #555; font-weight: 600; white-space: nowrap; }
td { word-break: break-all; }
td.num { text-align: right; font-variant-numeric: tabular-nums; }
.mono { font-family: ui-monospace, monospace; font-size: 0.9em; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 0.8em; font-weight: 600; }
.badge-yes { background: #dcffe4; color: #1a7f37; }
.badge-no { background: #ffebe9; color: #cf222e; }
.nav { margin-top: 20px; font-size: 0.9em; }
.nav a { color: #0366d6; text-decoration: none; }
.nav a:hover { text-decoration: underline; }
a { color: #0366d6; text-decoration: none; }
a:hover { text-decoration: underline; }
</style></head><body>
<h1>Info — <a href="/%s/">%s</a></h1>
<div class="nav" style="margin-top:-6px;margin-bottom:16px"><a href="/">Library</a></div>`,
		html.EscapeString(ze.title),
		html.EscapeString(slug), html.EscapeString(ze.title))

	// Header / Format
	fmt.Fprint(w, `<h2>Format</h2><table>`)
	fmt.Fprintf(w, `<tr><th>Filename</th><td>%s</td></tr>`, html.EscapeString(ze.filename))
	fmt.Fprintf(w, `<tr><th>UUID</th><td class="mono">%s</td></tr>`, uuidStr)
	fmt.Fprintf(w, `<tr><th>ZIM Version</th><td>%d.%d</td></tr>`, a.MajorVersion(), a.MinorVersion())
	fmt.Fprintf(w, `<tr><th>Entry Count</th><td>%d</td></tr>`, a.EntryCount())
	fmt.Fprintf(w, `<tr><th>Cluster Count</th><td>%d</td></tr>`, a.ClusterCount())

	yesNo := func(b bool) string {
		if b {
			return `<span class="badge badge-yes">Yes</span>`
		}
		return `<span class="badge badge-no">No</span>`
	}
	fmt.Fprintf(w, `<tr><th>Has Main Entry</th><td>%s</td></tr>`, yesNo(a.HasMainEntry()))
	if a.HasMainEntry() {
		if main, err := a.MainEntry(); err == nil {
			resolved, _ := main.Resolve()
			fmt.Fprintf(w, `<tr><th>Main Page</th><td><a href="/%s/%s">%s</a></td></tr>`,
				html.EscapeString(slug), html.EscapeString(resolved.Path()), html.EscapeString(resolved.Title()))
		}
	}
	fmt.Fprintf(w, `<tr><th>Full-text Index</th><td>%s</td></tr>`, yesNo(a.HasFulltextIndex()))
	fmt.Fprintf(w, `<tr><th>Title Index</th><td>%s</td></tr>`, yesNo(a.HasTitleIndex()))
	fmt.Fprint(w, `</table>`)

	// Metadata
	if len(metadata) > 0 {
		fmt.Fprint(w, `<h2>Metadata</h2><table>`)
		for _, m := range metadata {
			val := html.EscapeString(m.Value)
			// Wrap long values
			if len(m.Value) > 200 {
				val = `<div style="max-height:100px;overflow-y:auto">` + val + `</div>`
			}
			fmt.Fprintf(w, `<tr><th>%s</th><td>%s</td></tr>`, html.EscapeString(m.Key), val)
		}
		fmt.Fprint(w, `</table>`)
	}

	// Namespaces
	fmt.Fprint(w, `<h2>Namespaces</h2><table><tr><th>Namespace</th><th>Description</th><th style="text-align:right">Entries</th></tr>`)
	nsNames := map[byte]string{
		'C': "Content",
		'M': "Metadata",
		'X': "Indexes / Special",
		'W': "Well-known",
		'V': "User content (deprecated)",
		'A': "Articles (legacy ZIM v5)",
		'I': "Images (legacy ZIM v5)",
		'-': "Misc (legacy ZIM v5)",
	}
	for _, nc := range nsCounts {
		desc := nsNames[nc.NS]
		if desc == "" {
			desc = "Other"
		}
		fmt.Fprintf(w, `<tr><td class="mono">%c</td><td>%s</td><td class="num"><a href="/%s/-/info/ns?ns=%c">%d</a></td></tr>`,
			nc.NS, html.EscapeString(desc), html.EscapeString(slug), nc.NS, nc.Count)
	}
	fmt.Fprint(w, `</table>`)

	// MIME types in C namespace
	if len(mimeList) > 0 {
		fmt.Fprint(w, `<h2>Content Types (C namespace)</h2><table><tr><th>MIME Type</th><th style="text-align:right">Count</th></tr>`)
		for _, mc := range mimeList {
			fmt.Fprintf(w, `<tr><td class="mono">%s</td><td class="num"><a href="/%s/-/info/mime?type=%s">%d</a></td></tr>`,
				html.EscapeString(mc.MIME), html.EscapeString(slug), html.EscapeString(mc.MIME), mc.Count)
		}
		if redirectCount > 0 {
			fmt.Fprintf(w, `<tr><td><em>Redirects</em></td><td class="num"><a href="/%s/-/info/mime?type=redirect">%d</a></td></tr>`,
				html.EscapeString(slug), redirectCount)
		}
		fmt.Fprint(w, `</table>`)
	}

	// MIME type list (registered in header)
	mimeTypes := a.MIMETypes()
	if len(mimeTypes) > 0 {
		fmt.Fprint(w, `<h2>Registered MIME Types</h2><table><tr><th style="width:60px">Index</th><th>MIME Type</th></tr>`)
		for i, m := range mimeTypes {
			fmt.Fprintf(w, `<tr><td>%d</td><td class="mono">%s</td></tr>`, i, html.EscapeString(m))
		}
		fmt.Fprint(w, `</table>`)
	}

	fmt.Fprintf(w, `<div class="nav"><a href="/">Library</a> · <a href="/%s/">Main page</a> · <a href="/%s/-/search">Search</a> · <a href="/%s/-/browse">Browse</a></div>`,
		html.EscapeString(slug), html.EscapeString(slug), html.EscapeString(slug))
	fmt.Fprint(w, `</body></html>`)
}

// infoPageCSS is the shared stylesheet for info drill-down pages.
const infoPageCSS = `
body { font-family: system-ui, sans-serif; max-width: 900px; margin: 40px auto; padding: 0 20px; }
h1 { font-size: 1.4em; border-bottom: 1px solid #ddd; padding-bottom: 10px; }
table { border-collapse: collapse; width: 100%%; margin-bottom: 16px; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #eee; }
th { color: #555; font-weight: 600; }
td.mono { font-family: ui-monospace, monospace; font-size: 0.9em; }
td.num { text-align: right; font-variant-numeric: tabular-nums; }
.badge { display: inline-block; padding: 2px 6px; border-radius: 4px; font-size: 0.75em; font-weight: 600; }
.badge-redirect { background: #fff8c5; color: #735c0f; }
.badge-mime { background: #ddf4ff; color: #0969da; }
a { color: #0366d6; text-decoration: none; }
a:hover { text-decoration: underline; }
.pager { margin-top: 16px; font-size: 0.9em; }
.pager a { margin-right: 12px; }
.nav { margin-top: 20px; font-size: 0.9em; }
.count { color: #666; font-size: 0.9em; margin-bottom: 12px; }
`

func parseOffsetLimit(r *http.Request) (int, int) {
	offset := 0
	limit := 100
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	return offset, limit
}

// handleInfoNamespace serves GET /{slug}/-/info/ns?ns=C — lists entries in a namespace.
func (lib *library) handleInfoNamespace(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	nsStr := r.URL.Query().Get("ns")
	if len(nsStr) != 1 {
		http.Error(w, "ns parameter must be a single character", http.StatusBadRequest)
		return
	}
	ns := nsStr[0]
	offset, limit := parseOffsetLimit(r)

	a := ze.archive
	total := a.EntryCountByNamespace(ns)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Namespace %c — %s</title>
<style>%s</style></head><body>
<h1>Namespace <code>%c</code> — <a href="/%s/-/info">%s</a></h1>
<div class="nav" style="margin-top:-6px;margin-bottom:16px"><a href="/">Library</a> · <a href="/%s/-/info">Info</a></div>
<p class="count">%d entries total</p>
<table><tr><th style="width:50%%">Path</th><th>Title</th><th>Type</th></tr>`,
		ns, html.EscapeString(ze.title),
		infoPageCSS,
		ns, html.EscapeString(slug), html.EscapeString(ze.title),
		html.EscapeString(slug),
		total)

	// Iterate entries in this namespace, applying offset/limit
	count := 0
	shown := 0
	for e := range a.EntriesByNamespace(ns) {
		if count < offset {
			count++
			continue
		}
		if shown >= limit {
			break
		}

		typeCell := ""
		if e.IsRedirect() {
			typeCell = `<span class="badge badge-redirect">redirect</span>`
		} else {
			mime := e.MIMEType()
			if mime != "" {
				typeCell = fmt.Sprintf(`<span class="badge badge-mime">%s</span>`, html.EscapeString(mime))
			}
		}

		path := e.Path()
		// Link to the entry detail page
		entryLink := fmt.Sprintf("/%s/-/info/entry?idx=%d", html.EscapeString(slug), e.Index())
		// If it's a C-namespace content entry, also link to the live content
		var pathCell string
		if ns == 'C' && !e.IsRedirect() {
			pathCell = fmt.Sprintf(`<a href="/%s/%s">%s</a> <a href="%s" style="font-size:0.8em;color:#888" title="Entry details">&#x2139;&#xFE0E;</a>`,
				html.EscapeString(slug), html.EscapeString(path), html.EscapeString(path), entryLink)
		} else {
			pathCell = fmt.Sprintf(`<a href="%s">%s</a>`, entryLink, html.EscapeString(e.FullPath()))
		}

		fmt.Fprintf(w, `<tr><td class="mono">%s</td><td>%s</td><td>%s</td></tr>`,
			pathCell, html.EscapeString(e.Title()), typeCell)
		count++
		shown++
	}

	fmt.Fprint(w, `</table>`)

	// Pager
	fmt.Fprint(w, `<div class="pager">`)
	if offset > 0 {
		prev := offset - limit
		if prev < 0 {
			prev = 0
		}
		fmt.Fprintf(w, `<a href="/%s/-/info/ns?ns=%c&offset=%d&limit=%d">Previous</a>`,
			html.EscapeString(slug), ns, prev, limit)
	}
	if offset+shown < total {
		fmt.Fprintf(w, `<a href="/%s/-/info/ns?ns=%c&offset=%d&limit=%d">Next</a>`,
			html.EscapeString(slug), ns, offset+limit, limit)
	}
	fmt.Fprint(w, `</div>`)

	fmt.Fprintf(w, `<div class="nav"><a href="/%s/-/info">Back to info</a></div>`,
		html.EscapeString(slug))
	fmt.Fprint(w, `</body></html>`)
}

// handleInfoMIME serves GET /{slug}/-/info/mime?type=text/html — lists C entries by MIME type.
func (lib *library) handleInfoMIME(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	mimeFilter := r.URL.Query().Get("type")
	if mimeFilter == "" {
		http.Error(w, "type parameter required", http.StatusBadRequest)
		return
	}
	isRedirect := mimeFilter == "redirect"
	offset, limit := parseOffsetLimit(r)

	a := ze.archive

	// Single pass: collect matching entries for the page window and total count.
	type matchEntry struct {
		index    uint32
		path     string
		fullPath string
		title    string
		redirect bool
	}
	var matches []matchEntry
	total := 0
	for e := range a.EntriesByTitlePrefix('C', "") {
		match := false
		if isRedirect {
			match = e.IsRedirect()
		} else {
			match = !e.IsRedirect() && e.MIMEType() == mimeFilter
		}
		if !match {
			continue
		}
		if total >= offset && len(matches) < limit {
			matches = append(matches, matchEntry{
				index:    e.Index(),
				path:     e.Path(),
				fullPath: e.FullPath(),
				title:    e.Title(),
				redirect: e.IsRedirect(),
			})
		}
		total++
	}

	heading := mimeFilter
	if isRedirect {
		heading = "Redirects"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s — %s</title>
<style>%s</style></head><body>
<h1><code>%s</code> — <a href="/%s/-/info">%s</a></h1>
<div class="nav" style="margin-top:-6px;margin-bottom:16px"><a href="/">Library</a> · <a href="/%s/-/info">Info</a></div>
<p class="count">%d entries total</p>
<table><tr><th style="width:60%%">Path</th><th>Title</th></tr>`,
		html.EscapeString(heading), html.EscapeString(ze.title),
		infoPageCSS,
		html.EscapeString(heading), html.EscapeString(slug), html.EscapeString(ze.title),
		html.EscapeString(slug),
		total)

	for _, m := range matches {
		entryLink := fmt.Sprintf("/%s/-/info/entry?idx=%d", html.EscapeString(slug), m.index)
		var pathCell string
		if !m.redirect && (mimeFilter == "text/html" || strings.HasPrefix(mimeFilter, "image/")) {
			pathCell = fmt.Sprintf(`<a href="/%s/%s">%s</a> <a href="%s" style="font-size:0.8em;color:#888" title="Entry details">&#x2139;&#xFE0E;</a>`,
				html.EscapeString(slug), html.EscapeString(m.path), html.EscapeString(m.path), entryLink)
		} else {
			pathCell = fmt.Sprintf(`<a href="%s">%s</a>`, entryLink, html.EscapeString(m.fullPath))
		}

		fmt.Fprintf(w, `<tr><td class="mono">%s</td><td>%s</td></tr>`,
			pathCell, html.EscapeString(m.title))
	}

	fmt.Fprint(w, `</table>`)

	// Pager
	fmt.Fprint(w, `<div class="pager">`)
	if offset > 0 {
		prev := offset - limit
		if prev < 0 {
			prev = 0
		}
		fmt.Fprintf(w, `<a href="/%s/-/info/mime?type=%s&offset=%d&limit=%d">Previous</a>`,
			html.EscapeString(slug), html.EscapeString(mimeFilter), prev, limit)
	}
	if offset+len(matches) < total {
		fmt.Fprintf(w, `<a href="/%s/-/info/mime?type=%s&offset=%d&limit=%d">Next</a>`,
			html.EscapeString(slug), html.EscapeString(mimeFilter), offset+limit, limit)
	}
	fmt.Fprint(w, `</div>`)

	fmt.Fprintf(w, `<div class="nav"><a href="/%s/-/info">Back to info</a></div>`,
		html.EscapeString(slug))
	fmt.Fprint(w, `</body></html>`)
}

// handleInfoEntry serves GET /{slug}/-/info/entry?idx=42 — detail page for a single entry.
func (lib *library) handleInfoEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	idxStr := r.URL.Query().Get("idx")
	idx, err := strconv.ParseUint(idxStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid idx parameter", http.StatusBadRequest)
		return
	}

	a := ze.archive
	e, err := a.EntryByIndex(uint32(idx))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Entry %d — %s</title>
<style>%s
th { width: 160px; }
</style></head><body>
<h1>Entry #%d — <a href="/%s/-/info">%s</a></h1>
<div class="nav" style="margin-top:-6px;margin-bottom:16px"><a href="/">Library</a> · <a href="/%s/-/info">Info</a></div>
<table>`,
		idx, html.EscapeString(ze.title),
		infoPageCSS,
		idx, html.EscapeString(slug), html.EscapeString(ze.title),
		html.EscapeString(slug))

	fmt.Fprintf(w, `<tr><th>Index</th><td class="num">%d</td></tr>`, idx)
	fmt.Fprintf(w, `<tr><th>Full Path</th><td class="mono">%s</td></tr>`, html.EscapeString(e.FullPath()))
	fmt.Fprintf(w, `<tr><th>Namespace</th><td class="mono">%c</td></tr>`, e.Namespace())
	fmt.Fprintf(w, `<tr><th>Path</th><td class="mono">%s</td></tr>`, html.EscapeString(e.Path()))
	fmt.Fprintf(w, `<tr><th>Title</th><td>%s</td></tr>`, html.EscapeString(e.Title()))
	fmt.Fprintf(w, `<tr><th>Is Redirect</th><td>%v</td></tr>`, e.IsRedirect())

	if e.IsRedirect() {
		if target, err := e.RedirectTarget(); err == nil {
			targetLink := fmt.Sprintf("/%s/-/info/entry?idx=%d", html.EscapeString(slug), target.Index())
			fmt.Fprintf(w, `<tr><th>Redirect Target</th><td><a href="%s">%s</a> (index %d)</td></tr>`,
				targetLink, html.EscapeString(target.FullPath()), target.Index())
		}
		if resolved, err := e.Resolve(); err == nil {
			resolvedLink := fmt.Sprintf("/%s/-/info/entry?idx=%d", html.EscapeString(slug), resolved.Index())
			fmt.Fprintf(w, `<tr><th>Resolves To</th><td><a href="%s">%s</a> (index %d)</td></tr>`,
				resolvedLink, html.EscapeString(resolved.FullPath()), resolved.Index())
		}
	} else {
		mime := e.MIMEType()
		if mime != "" {
			fmt.Fprintf(w, `<tr><th>MIME Type</th><td class="mono">%s</td></tr>`, html.EscapeString(mime))
		}
		if size, err := e.ContentSize(); err == nil {
			fmt.Fprintf(w, `<tr><th>Content Size</th><td class="num">%s</td></tr>`, formatBytes(size))
		}
	}

	fmt.Fprint(w, `</table>`)

	// Action links
	fmt.Fprint(w, `<div style="margin-top:16px">`)
	if e.Namespace() == 'C' {
		if e.IsRedirect() {
			if resolved, err := e.Resolve(); err == nil {
				fmt.Fprintf(w, `<a href="/%s/%s">View content (follows redirect)</a>`,
					html.EscapeString(slug), html.EscapeString(resolved.Path()))
			}
		} else {
			fmt.Fprintf(w, `<a href="/%s/%s">View content</a>`,
				html.EscapeString(slug), html.EscapeString(e.Path()))
		}
	}
	fmt.Fprint(w, `</div>`)

	// Navigation: prev/next entries
	fmt.Fprint(w, `<div class="pager">`)
	if idx > 0 {
		fmt.Fprintf(w, `<a href="/%s/-/info/entry?idx=%d">Previous entry (#%d)</a>`,
			html.EscapeString(slug), idx-1, idx-1)
	}
	if uint32(idx+1) < a.EntryCount() {
		fmt.Fprintf(w, `<a href="/%s/-/info/entry?idx=%d">Next entry (#%d)</a>`,
			html.EscapeString(slug), idx+1, idx+1)
	}
	fmt.Fprint(w, `</div>`)

	// Back link
	nsLink := fmt.Sprintf("/%s/-/info/ns?ns=%c", html.EscapeString(slug), e.Namespace())
	fmt.Fprintf(w, `<div class="nav"><a href="/%s/-/info">Back to info</a> · <a href="%s">Namespace %c</a></div>`,
		html.EscapeString(slug), nsLink, e.Namespace())
	fmt.Fprint(w, `</body></html>`)
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GiB (%d bytes)", float64(b)/float64(1<<30), b)
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB (%d bytes)", float64(b)/float64(1<<20), b)
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KiB (%d bytes)", float64(b)/float64(1<<10), b)
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}
