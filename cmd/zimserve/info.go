package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// handleInfo serves GET /{slug}/-/info
func (lib *library) handleInfo(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	a := ze.archive

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

	metaKeys := []string{
		"Title", "Creator", "Publisher", "Date", "Description",
		"LongDescription", "Language", "License", "Tags", "Relation",
		"Flavour", "Source", "Counter", "Scraper",
	}
	type metaEntry struct {
		Key   string
		Value string
	}
	var rawMeta []metaEntry
	for _, key := range metaKeys {
		val, err := a.Metadata(key)
		if err == nil && val != "" {
			rawMeta = append(rawMeta, metaEntry{key, val})
		}
	}

	uuid := a.UUID()
	uuidStr := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])

	cs := a.CacheStats()
	hitRate := "—"
	if total := cs.Hits + cs.Misses; total > 0 {
		hitRate = fmt.Sprintf("%.1f%%", 100*float64(cs.Hits)/float64(total))
	}

	mainPath, mainTitle := "", ""
	if a.HasMainEntry() {
		if main, err := a.MainEntry(); err == nil {
			if resolved, err := main.Resolve(); err == nil {
				mainPath = resolved.Path()
				mainTitle = resolved.Title()
			}
		}
	}

	metadata := make([]infoMetaRow, len(rawMeta))
	for i, m := range rawMeta {
		metadata[i] = infoMetaRow{Key: m.Key, Value: m.Value, Long: len(m.Value) > 200}
	}

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
	namespaces := make([]infoNSRow, 0, len(nsCounts))
	for _, nc := range nsCounts {
		desc := nsNames[nc.NS]
		if desc == "" {
			desc = "Other"
		}
		namespaces = append(namespaces, infoNSRow{
			NS:    string(rune(nc.NS)),
			Desc:  desc,
			Count: nc.Count,
		})
	}

	mimeCountRows := make([]infoMIMECountRow, len(mimeList))
	for i, mc := range mimeList {
		mimeCountRows[i] = infoMIMECountRow(mc)
	}

	regMIMETypes := a.MIMETypes()
	mimeTypeRows := make([]infoMIMETypeRow, len(regMIMETypes))
	for i, m := range regMIMETypes {
		mimeTypeRows[i] = infoMIMETypeRow{Index: i, MIME: m}
	}

	renderWith(w, tmplInfo, infoData{
		Slug:         slug,
		Title:        ze.title,
		Filename:     ze.filename,
		UUID:         uuidStr,
		MajorVersion: a.MajorVersion(),
		MinorVersion: a.MinorVersion(),
		EntryCount:   a.EntryCount(),
		ClusterCount: a.ClusterCount(),
		Cache: infoCacheData{
			Size:     cs.Size,
			Capacity: cs.Capacity,
			HitRate:  hitRate,
			Hits:     cs.Hits,
			Misses:   cs.Misses,
			Bytes:    formatBytes(cs.Bytes),
		},
		HasMainEntry:  a.HasMainEntry(),
		MainPath:      mainPath,
		MainTitle:     mainTitle,
		FulltextIndex: infoIndexData{Has: a.HasFulltextIndex(), Format: a.FulltextIndexFormat()},
		TitleIndex:    infoIndexData{Has: a.HasTitleIndex(), Format: a.TitleIndexFormat()},
		Metadata:      metadata,
		Namespaces:    namespaces,
		MIMECounts:    mimeCountRows,
		RedirectCount: redirectCount,
		MIMETypes:     mimeTypeRows,
	})
}

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

// handleInfoNamespace serves GET /{slug}/-/info/ns?ns=C[&type=text/html]
func (lib *library) handleInfoNamespace(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	nsStr := r.URL.Query().Get("ns")
	if len(nsStr) != 1 {
		writeBadRequest(w, "The \u2018ns\u2019 parameter must be a single character.")
		return
	}
	ns := nsStr[0]
	typeFilter := r.URL.Query().Get("type")
	offset, limit := parseOffsetLimit(r)

	a := ze.archive

	type nsRow struct {
		index     uint32
		path      string
		fullPath  string
		title     string
		redirect  bool
		mime      string
		isContent bool
	}

	var rows []nsRow
	var total int

	if typeFilter == "" {
		total = a.EntryCountByNamespace(ns)
		skipped := 0
		for e := range a.EntriesByNamespace(ns) {
			if skipped < offset {
				skipped++
				continue
			}
			if len(rows) >= limit {
				break
			}
			mime := ""
			if !e.IsRedirect() {
				mime = e.MIMEType()
			}
			rows = append(rows, nsRow{
				index:     e.Index(),
				path:      e.Path(),
				fullPath:  e.FullPath(),
				title:     e.Title(),
				redirect:  e.IsRedirect(),
				mime:      mime,
				isContent: ns == 'C' && !e.IsRedirect(),
			})
		}
	} else {
		isRedirect := typeFilter == "redirect"
		for e := range a.EntriesByNamespace(ns) {
			if isRedirect {
				if !e.IsRedirect() {
					continue
				}
			} else {
				if e.IsRedirect() || e.MIMEType() != typeFilter {
					continue
				}
			}
			if total >= offset && len(rows) < limit {
				mime := ""
				if !e.IsRedirect() {
					mime = e.MIMEType()
				}
				rows = append(rows, nsRow{
					index:     e.Index(),
					path:      e.Path(),
					fullPath:  e.FullPath(),
					title:     e.Title(),
					redirect:  e.IsRedirect(),
					mime:      mime,
					isContent: ns == 'C' && !e.IsRedirect(),
				})
			}
			total++
		}
	}

	rowItems := make([]infoNSRowItem, len(rows))
	for i, row := range rows {
		rowItems[i] = infoNSRowItem{
			Index:      row.index,
			Path:       row.path,
			FullPath:   row.fullPath,
			Title:      row.title,
			IsRedirect: row.redirect,
			IsContent:  row.isContent,
			MIME:       row.mime,
		}
	}

	renderWith(w, tmplInfoNS, infoNSData{
		Slug:       slug,
		Title:      ze.title,
		NS:         nsStr,
		TypeFilter: typeFilter,
		MIMETypes:  a.MIMETypes(),
		Total:      total,
		Rows:       rowItems,
		HasPrev:    offset > 0,
		HasNext:    offset+len(rows) < total,
		PrevOffset: max(offset-limit, 0),
		NextOffset: offset + limit,
		Limit:      limit,
	})
}

// handleInfoMIME serves GET /{slug}/-/info/mime?type=text/html
func (lib *library) handleInfoMIME(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	mimeFilter := r.URL.Query().Get("type")
	if mimeFilter == "" {
		writeBadRequest(w, "A \u2018type\u2019 parameter is required.")
		return
	}
	isRedirect := mimeFilter == "redirect"
	offset, limit := parseOffsetLimit(r)

	a := ze.archive

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

	matchRows := make([]infoMIMEMatch, len(matches))
	for i, m := range matches {
		var contentLink string
		if !m.redirect && (mimeFilter == "text/html" || strings.HasPrefix(mimeFilter, "image/")) {
			contentLink = "/" + slug + "/" + m.path
		}
		matchRows[i] = infoMIMEMatch{
			Index:       m.index,
			Path:        m.path,
			FullPath:    m.fullPath,
			Title:       m.title,
			ContentLink: contentLink,
		}
	}

	renderWith(w, tmplInfoMIME, infoMIMEData{
		Slug:       slug,
		Title:      ze.title,
		Heading:    heading,
		MIMEFilter: mimeFilter,
		Total:      total,
		Matches:    matchRows,
		HasPrev:    offset > 0,
		HasNext:    offset+len(matches) < total,
		PrevOffset: max(offset-limit, 0),
		NextOffset: offset + limit,
		Limit:      limit,
	})
}

// handleInfoEntry serves GET /{slug}/-/info/entry?idx=42
func (lib *library) handleInfoEntry(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	idxStr := r.URL.Query().Get("idx")
	idx, err := strconv.ParseUint(idxStr, 10, 32)
	if err != nil {
		writeBadRequest(w, "Invalid \u2018idx\u2019 parameter.")
		return
	}

	a := ze.archive
	e, err := a.EntryByIndex(uint32(idx))
	if err != nil {
		write404(w)
		return
	}

	data := infoEntryData{
		Slug:         slug,
		ArchiveTitle: ze.title,
		EntryIdx:     idx,
		FullPath:     e.FullPath(),
		NS:           string(rune(e.Namespace())),
		Path:         e.Path(),
		EntryTitle:   e.Title(),
		IsRedirect:   e.IsRedirect(),
		HasPrev:      idx > 0,
		HasNext:      idx+1 < uint64(a.EntryCount()),
	}
	if data.HasPrev {
		data.PrevIdx = idx - 1
	}
	if data.HasNext {
		data.NextIdx = idx + 1
	}

	if e.IsRedirect() {
		if target, err := e.RedirectTarget(); err == nil {
			data.RedirectTarget = &entryRef{Index: target.Index(), FullPath: target.FullPath()}
		}
		if resolved, err := e.Resolve(); err == nil {
			data.ResolvesTo = &entryRef{Index: resolved.Index(), FullPath: resolved.FullPath()}
		}
	} else {
		data.MIME = e.MIMEType()
		if size, err := e.ContentSize(); err == nil {
			data.HasSize = true
			data.ContentSize = formatBytes(size)
		}
	}

	if e.Namespace() == 'C' {
		if e.IsRedirect() {
			if resolved, err := e.Resolve(); err == nil {
				data.ViewLink = "/" + slug + "/" + resolved.Path()
			}
		} else {
			data.ViewLink = "/" + slug + "/" + e.Path()
		}
	}

	renderWith(w, tmplInfoEntry, data)
}

// handleInfoCluster serves GET /{slug}/-/info/cluster[?n=X]
func (lib *library) handleInfoCluster(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}
	a := ze.archive

	nStr := r.URL.Query().Get("n")
	if nStr != "" {
		n64, err := strconv.ParseUint(nStr, 10, 32)
		if err != nil || uint32(n64) >= a.ClusterCount() {
			writeBadRequest(w, "Invalid cluster number.")
			return
		}
		n := uint32(n64)

		meta, err := a.ClusterMetaAt(n)
		if err != nil {
			log.Printf("error reading cluster %d for %s: %v", n64, slug, err)
			write500(w)
			return
		}

		data := infoClusterDetailData{
			Slug:           slug,
			ArchiveTitle:   ze.title,
			N:              n,
			ClusterCount:   a.ClusterCount(),
			Offset:         meta.Offset,
			CompressedSize: formatBytes(int64(meta.CompressedSize)),
			Compression:    meta.Compression,
			Extended:       meta.Extended,
			HasPrev:        n > 0,
			HasNext:        n+1 < a.ClusterCount(),
		}
		if data.HasPrev {
			data.PrevN = n - 1
		}
		if data.HasNext {
			data.NextN = n + 1
		}

		blobSizes, blobErr := a.ClusterBlobSizes(n)
		if blobErr != nil {
			data.BlobError = blobErr.Error()
		} else {
			totalDecomp := int64(0)
			blobs := make([]infoBlob, len(blobSizes))
			for i, s := range blobSizes {
				totalDecomp += int64(s)
				blobs[i] = infoBlob{Index: i, Size: formatBytes(int64(s))}
			}
			data.BlobCount = len(blobSizes)
			data.TotalDecomp = formatBytes(totalDecomp)
			data.Blobs = blobs
		}

		entries, entErr := a.EntriesInCluster(n)
		if entErr != nil {
			data.EntryError = entErr.Error()
		} else {
			clusterEntries := make([]infoClusterEntry, len(entries))
			for i, e := range entries {
				clusterEntries[i] = infoClusterEntry{
					Index:     e.Index(),
					FullPath:  e.FullPath(),
					Path:      e.Path(),
					Title:     e.Title(),
					IsContent: e.Namespace() == 'C',
				}
			}
			data.Entries = clusterEntries
		}

		renderWith(w, tmplInfoClusterDetail, data)
		return
	}

	// List view
	offset, limit := parseOffsetLimit(r)
	total := int(a.ClusterCount())
	end := min(offset+limit, total)

	rows := make([]infoClusterRow, 0, end-offset)
	for i := offset; i < end; i++ {
		meta, err := a.ClusterMetaAt(uint32(i))
		if err != nil {
			rows = append(rows, infoClusterRow{Num: uint32(i), HasError: true, ErrMsg: err.Error()})
		} else {
			rows = append(rows, infoClusterRow{
				Num:      uint32(i),
				Offset:   meta.Offset,
				Size:     formatBytes(int64(meta.CompressedSize)),
				Comp:     meta.Compression,
				Extended: meta.Extended,
			})
		}
	}

	renderWith(w, tmplInfoClusterList, infoClusterListData{
		Slug:       slug,
		Title:      ze.title,
		Total:      total,
		Rows:       rows,
		HasPrev:    offset > 0,
		HasNext:    end < total,
		PrevOffset: max(offset-limit, 0),
		NextOffset: offset + limit,
		Limit:      limit,
	})
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
