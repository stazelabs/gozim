package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stazelabs/gozim/zim"
)

func (lib *library) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		write404(w)
		return
	}
	lib.writeIndexPage(w, r)
}

type searchResult struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// searchPrefixes returns capitalization variants to try via binary search.
func searchPrefixes(q string) []string {
	seen := make(map[string]bool)
	var prefixes []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			prefixes = append(prefixes, s)
		}
	}
	add(q)
	if r, size := utf8.DecodeRuneInString(q); r != utf8.RuneError {
		add(string(unicode.ToUpper(r)) + q[size:])
		add(string(unicode.ToLower(r)) + q[size:])
	}
	return prefixes
}

func searchArchive(a *zim.Archive, slug, q string, results *[]searchResult, limit int) {
	seen := make(map[string]bool)
	for _, prefix := range searchPrefixes(q) {
		for e := range a.EntriesByTitlePrefix('C', prefix) {
			if mime := e.MIMEType(); mime != "text/html" && mime != "" {
				continue
			}
			path := e.Path()
			if seen[path] {
				continue
			}
			seen[path] = true
			*results = append(*results, searchResult{
				Path:  "/" + slug + "/" + path,
				Title: e.Title(),
			})
			if len(*results) >= limit {
				return
			}
		}
	}
}

func (lib *library) handleSearchAll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	var results []searchResult
	limit := 20
	for _, slug := range lib.slugs {
		ze := lib.archives[slug]
		searchArchive(ze.archive, slug, q, &results, limit)
		if len(results) >= limit {
			break
		}
	}
	if results == nil {
		results = []searchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (lib *library) handleSearchJSON(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
		return
	}

	var results []searchResult
	searchArchive(ze.archive, slug, q, &results, 20)
	if results == nil {
		results = []searchResult{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleSearchPage serves GET /{slug}/-/search?q=term&limit=25[&format=json]
func (lib *library) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	q := r.URL.Query().Get("q")
	format := r.URL.Query().Get("format")
	limitStr := r.URL.Query().Get("limit")
	limit := 25
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	if format == "json" {
		var results []searchResult
		if q != "" {
			searchArchive(ze.archive, slug, q, &results, limit)
		}
		if results == nil {
			results = []searchResult{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
		return
	}

	var results []searchResult
	if q != "" {
		searchArchive(ze.archive, slug, q, &results, limit)
	}

	renderWith(w, tmplSearch, searchData{
		Slug:     slug,
		Title:    ze.title,
		Query:    q,
		HasQuery: q != "",
		Results:  results,
	})
}

// randomArticle picks a random text/html entry from the archive.
func randomArticle(a *zim.Archive) (zim.Entry, error) {
	for range 50 {
		e, err := a.RandomEntry('C')
		if err != nil {
			return zim.Entry{}, err
		}
		resolved, err := e.Resolve()
		if err != nil {
			continue
		}
		if resolved.MIMEType() == "text/html" {
			return resolved, nil
		}
	}
	return zim.Entry{}, fmt.Errorf("no HTML article found after retries")
}

// handleRandom serves GET /{slug}/-/random
func (lib *library) handleRandom(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	entry, err := randomArticle(ze.archive)
	if err != nil {
		log.Printf("error getting random article for %s: %v", slug, err)
		writeErrorPage(w, http.StatusNotFound, "No articles available", "This ZIM file has no HTML articles to browse.")
		return
	}

	http.Redirect(w, r, "/"+slug+"/"+entry.Path(), http.StatusFound)
}

// handleRandomAll serves GET /_random
func (lib *library) handleRandomAll(w http.ResponseWriter, r *http.Request) {
	slug := lib.slugs[rand.IntN(len(lib.slugs))]
	ze := lib.archives[slug]

	entry, err := randomArticle(ze.archive)
	if err != nil {
		log.Printf("error getting random article for %s: %v", slug, err)
		writeErrorPage(w, http.StatusNotFound, "No articles available", "None of the loaded ZIM files have HTML articles to browse.")
		return
	}

	http.Redirect(w, r, "/"+slug+"/"+entry.Path(), http.StatusFound)
}

// handleBrowse serves GET /{slug}/-/browse?letter=A[&offset=0&limit=50]
func (lib *library) handleBrowse(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	letter := r.URL.Query().Get("letter")
	offset := 0
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	totalC := ze.archive.EntryCountByNamespace('C')

	// Build letter counts (O(26 log N) binary searches).
	letterCounts := make(map[byte]int, 26)
	for c := byte('A'); c <= 'Z'; c++ {
		letterCounts[c] = ze.archive.TitlePrefixCount('C', string(c)) +
			ze.archive.TitlePrefixCount('C', strings.ToLower(string(c)))
	}

	letters := make([]browseLetterInfo, 0, 26)
	for c := byte('A'); c <= 'Z'; c++ {
		letters = append(letters, browseLetterInfo{
			L:      string(c),
			Empty:  letterCounts[c] == 0,
			Active: letter == string(c),
		})
	}

	data := browseData{
		Slug:       slug,
		Title:      ze.title,
		TotalC:     totalC,
		Letters:    letters,
		HashActive: letter == "#",
		HasLetter:  letter != "",
		Letter:     letter,
	}

	if letter != "" {
		var letterCount int
		var pageEntries []searchResult

		if letter == "#" {
			var all []searchResult
			for e := range ze.archive.EntriesByTitlePrefix('C', "") {
				t := e.Title()
				if t == "" {
					continue
				}
				ru, _ := utf8.DecodeRuneInString(t)
				if !unicode.IsLetter(ru) {
					all = append(all, searchResult{
						Path:  "/" + slug + "/" + e.Path(),
						Title: t,
					})
				}
			}
			letterCount = len(all)
			if offset < letterCount {
				end := min(offset+limit, letterCount)
				pageEntries = all[offset:end]
			}
		} else {
			upper := strings.ToUpper(letter)
			lower := strings.ToLower(letter)
			letterCount = ze.archive.TitlePrefixCount('C', upper)
			if lower != upper {
				letterCount += ze.archive.TitlePrefixCount('C', lower)
			}
			var collected []searchResult
			for e := range ze.archive.EntriesByTitlePrefix('C', upper) {
				collected = append(collected, searchResult{
					Path:  "/" + slug + "/" + e.Path(),
					Title: e.Title(),
				})
				if len(collected) >= offset+limit {
					break
				}
			}
			if lower != upper && len(collected) < offset+limit {
				for e := range ze.archive.EntriesByTitlePrefix('C', lower) {
					collected = append(collected, searchResult{
						Path:  "/" + slug + "/" + e.Path(),
						Title: e.Title(),
					})
					if len(collected) >= offset+limit {
						break
					}
				}
			}
			if offset < len(collected) {
				end := min(offset+limit, len(collected))
				pageEntries = collected[offset:end]
			}
		}

		noEntries := letterCount == 0 || offset >= letterCount
		data.LetterCount = letterCount
		data.NoEntries = noEntries
		data.Entries = pageEntries

		if letterCount > 0 {
			data.HasPrev = offset > 0
			data.HasNext = offset+limit < letterCount
			data.PrevOffset = max(offset-limit, 0)
			data.NextOffset = offset + limit
			data.Limit = limit
			data.PageStart = offset + 1
			data.PageEnd = min(offset+limit, letterCount)
		}
	}

	renderWith(w, tmplBrowse, data)
}

// commaInt formats n with comma thousands separators.
func commaInt(n int) string {
	s := strconv.Itoa(n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func (lib *library) handleContent(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	contentPath := r.PathValue("path")

	ze, ok := lib.archives[slug]
	if !ok {
		write404(w)
		return
	}

	if contentPath == "" {
		if !ze.archive.HasMainEntry() {
			write404(w)
			return
		}
		main, err := ze.archive.MainEntry()
		if err != nil {
			log.Printf("error reading main entry for %s: %v", slug, err)
			write500(w)
			return
		}
		resolved, err := main.Resolve()
		if err != nil {
			log.Printf("error resolving main entry for %s: %v", slug, err)
			write500(w)
			return
		}
		http.Redirect(w, r, "/"+slug+"/"+resolved.Path(), http.StatusFound)
		return
	}

	if contentPath == "favicon.ico" {
		data, err := ze.archive.Illustration(48)
		if err != nil {
			write404(w)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
		return
	}

	entry, err := ze.archive.EntryByPath("C/" + contentPath)
	if err != nil {
		write404(w)
		return
	}

	if entry.IsRedirect() {
		resolved, err := entry.Resolve()
		if err != nil {
			log.Printf("error resolving redirect for %s/%s: %v", slug, contentPath, err)
			write500(w)
			return
		}
		http.Redirect(w, r, "/"+slug+"/"+resolved.Path(), http.StatusFound)
		return
	}

	etag := makeETag(ze, entry.FullPath())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	size, err := entry.ContentSize()
	if err != nil {
		log.Printf("error reading content for %s/%s: %v", slug, contentPath, err)
		write500(w)
		return
	}
	reader, err := entry.ContentReader()
	if err != nil {
		log.Printf("error reading content for %s/%s: %v", slug, contentPath, err)
		write500(w)
		return
	}

	mime := entry.MIMEType()
	if mime != "" {
		if strings.HasPrefix(mime, "text/") && !strings.Contains(mime, "charset") {
			mime += "; charset=utf-8"
		}
		w.Header().Set("Content-Type", mime)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)

	if entry.MIMEType() == "text/html" {
		body, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("error reading html content for %s/%s: %v", slug, contentPath, err)
			write500(w)
			return
		}
		bar := headerBarHTML(slug, ze.title, ze.archive)
		body = injectHeaderBar(body, []byte(bar))
		body = injectFooterBar(body, []byte(footerBarHTML()))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	io.Copy(w, reader)
}

// injectHeaderBar inserts the bar HTML after the opening <body...> tag.
func injectHeaderBar(body, bar []byte) []byte {
	lower := bytes.ToLower(body)
	idx := bytes.Index(lower, []byte("<body"))
	if idx == -1 {
		return append(bar, body...)
	}
	closeIdx := bytes.IndexByte(body[idx:], '>')
	if closeIdx == -1 {
		return append(bar, body...)
	}
	insertAt := idx + closeIdx + 1
	result := make([]byte, 0, len(body)+len(bar))
	result = append(result, body[:insertAt]...)
	result = append(result, bar...)
	result = append(result, body[insertAt:]...)
	return result
}

// footerBarHTML returns a self-contained HTML+CSS footer bar for injection into pages.
func footerBarHTML() string {
	return `<style>
#gzim-footer{position:fixed;bottom:0;left:0;right:0;z-index:999998;background:#f6f8fa;border-top:1px solid #d0d7de;padding:4px 12px;font:12px/1.4 system-ui,sans-serif;display:flex;align-items:center;justify-content:center;gap:8px;color:#666}
#gzim-footer a{color:#0366d6;text-decoration:none;display:inline-flex;align-items:center;gap:3px}
#gzim-footer a:hover{text-decoration:underline}
body{padding-bottom:32px!important}
</style>
<div id="gzim-footer"><a href="https://github.com/stazelabs/gozim"><svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>zimserve</a><span>·</span><a href="/_docs">Documentation</a><span>·</span><a href="https://github.com/stazelabs/gozim/blob/main/LICENSE">Apache 2.0</a></div>`
}

// injectFooterBar inserts the footer bar HTML before the closing </body> tag.
func injectFooterBar(body, bar []byte) []byte {
	lower := bytes.ToLower(body)
	idx := bytes.Index(lower, []byte("</body"))
	if idx == -1 {
		return append(body, bar...)
	}
	result := make([]byte, 0, len(body)+len(bar))
	result = append(result, body[:idx]...)
	result = append(result, bar...)
	result = append(result, body[idx:]...)
	return result
}

// headerBarHTML returns a self-contained HTML+CSS navigation bar for injection into ZIM pages.
func headerBarHTML(slug, title string, a *zim.Archive) string {
	letters := make([]barLetterInfo, 0, 26)
	for c := byte('A'); c <= 'Z'; c++ {
		count := a.TitlePrefixCount('C', string(c)) +
			a.TitlePrefixCount('C', strings.ToLower(string(c)))
		letters = append(letters, barLetterInfo{L: string(c), Active: count > 0})
	}
	return renderBarHTML(slug, title, letters)
}
