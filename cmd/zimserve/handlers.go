package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stazelabs/gozim/zim"
)

func (lib *library) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	lib.writeIndexPage(w, r)
}

type searchResult struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// searchPrefixes returns capitalization variants to try via binary search.
// This avoids the O(N) linear scan of EntriesByTitlePrefixFold.
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
	// Title Case first letter
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
			// Only include HTML articles, not JS/CSS/image assets.
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
		http.NotFound(w, r)
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

	// JSON format
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

	// HTML format
	var results []searchResult
	if q != "" {
		searchArchive(ze.archive, slug, q, &results, limit)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Search — %s</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 800px; margin: 40px auto; padding: 0 20px; }
h1 { font-size: 1.4em; border-bottom: 1px solid #ddd; padding-bottom: 10px; }
form { margin-bottom: 20px; display: flex; gap: 8px; }
form input[type=text] { flex: 1; padding: 8px 12px; font-size: 1em; border: 1px solid #ccc; border-radius: 4px; }
form input[type=text]:focus { outline: none; border-color: #0366d6; }
form button { padding: 8px 16px; font-size: 1em; border: 1px solid #ccc; border-radius: 4px; cursor: pointer; }
.results a { display: block; padding: 8px 0; border-bottom: 1px solid #eee; color: #0366d6; text-decoration: none; }
.results a:hover { text-decoration: underline; }
.empty { color: #666; font-style: italic; }
.nav { margin-top: 10px; font-size: 0.9em; }
.nav a { color: #0366d6; }
</style></head><body>
<h1>Search — <a href="/%s/">%s</a></h1>
<form method="get">
<input type="text" name="q" value="%s" placeholder="Search articles..." autofocus>
<button type="submit">Search</button>
</form>`,
		html.EscapeString(ze.title),
		html.EscapeString(slug), html.EscapeString(ze.title),
		html.EscapeString(q))

	if q != "" {
		if len(results) == 0 {
			fmt.Fprint(w, `<p class="empty">No results found.</p>`)
		} else {
			fmt.Fprintf(w, `<p>%d result(s):</p><div class="results">`, len(results))
			for _, res := range results {
				fmt.Fprintf(w, `<a href="%s">%s</a>`, html.EscapeString(res.Path), html.EscapeString(res.Title))
			}
			fmt.Fprint(w, `</div>`)
		}
	}

	fmt.Fprintf(w, `<div class="nav"><a href="/">Library</a> · <a href="/%s/">Back to main page</a> · <a href="/%s/-/browse">Browse</a> · <a href="/%s/-/random">Random article</a></div>`,
		html.EscapeString(slug), html.EscapeString(slug), html.EscapeString(slug))
	fmt.Fprint(w, `</body></html>`)
}

// randomArticle picks a random text/html entry from the archive, retrying up
// to 50 times to skip non-article entries (JS, CSS, images, etc.).
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

// handleRandom serves GET /{slug}/-/random — redirects to a random C-namespace article.
func (lib *library) handleRandom(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	entry, err := randomArticle(ze.archive)
	if err != nil {
		log.Printf("error getting random article for %s: %v", slug, err)
		http.Error(w, "no articles available", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/"+slug+"/"+entry.Path(), http.StatusFound)
}

// handleRandomAll serves GET /_random — picks a random ZIM, then a random article.
func (lib *library) handleRandomAll(w http.ResponseWriter, r *http.Request) {
	slug := lib.slugs[rand.Intn(len(lib.slugs))]
	ze := lib.archives[slug]

	entry, err := randomArticle(ze.archive)
	if err != nil {
		log.Printf("error getting random article for %s: %v", slug, err)
		http.Error(w, "no articles available", http.StatusNotFound)
		return
	}

	http.Redirect(w, r, "/"+slug+"/"+entry.Path(), http.StatusFound)
}

// handleBrowse serves GET /{slug}/-/browse?letter=A[&offset=0&limit=50]
func (lib *library) handleBrowse(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ze, ok := lib.archives[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}

	letter := r.URL.Query().Get("letter")
	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset := 0
	if offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	totalC := ze.archive.EntryCountByNamespace('C')

	// Pre-compute per-letter counts (A-Z) for nav rendering — O(26 log N).
	letterCounts := make(map[byte]int, 26)
	for c := byte('A'); c <= 'Z'; c++ {
		letterCounts[c] = ze.archive.TitlePrefixCount('C', string(c)) +
			ze.archive.TitlePrefixCount('C', strings.ToLower(string(c)))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Browse — %s</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 800px; margin: 40px auto; padding: 0 20px; }
h1 { font-size: 1.4em; border-bottom: 1px solid #ddd; padding-bottom: 10px; }
.total { color: #666; font-size: 0.9em; margin: -6px 0 16px; }
.letters { display: flex; flex-wrap: wrap; gap: 4px; margin-bottom: 20px; }
.letters a { display: inline-block; padding: 6px 10px; border: 1px solid #ccc; border-radius: 4px; text-decoration: none; color: #0366d6; font-weight: bold; }
.letters a:hover { background: #f6f8fa; }
.letters a.active { background: #0366d6; color: white; border-color: #0366d6; }
.letters span { display: inline-block; padding: 6px 10px; border: 1px solid #eee; border-radius: 4px; color: #ccc; font-weight: bold; cursor: default; }
.letter-info { color: #666; font-size: 0.9em; margin-bottom: 12px; }
.entries a { display: block; padding: 6px 0; border-bottom: 1px solid #eee; color: #0366d6; text-decoration: none; }
.entries a:hover { text-decoration: underline; }
.pager { display: flex; align-items: center; gap: 16px; margin-top: 16px; font-size: 0.9em; color: #666; }
.pager a { color: #0366d6; text-decoration: none; }
.pager a:hover { text-decoration: underline; }
.nav { margin-top: 24px; font-size: 0.9em; }
.nav a { color: #0366d6; }
</style></head><body>
<h1>Browse — <a href="/%s/">%s</a></h1>
<p class="total">%s articles total</p>
<div class="letters">`,
		html.EscapeString(ze.title),
		html.EscapeString(slug), html.EscapeString(ze.title),
		commaInt(totalC))

	// Letter navigation A-Z + #
	for c := byte('A'); c <= 'Z'; c++ {
		l := string(c)
		if letterCounts[c] == 0 {
			fmt.Fprintf(w, `<span>%s</span>`, l)
		} else if letter == l {
			fmt.Fprintf(w, `<a href="/%s/-/browse?letter=%s" class="active">%s</a>`,
				html.EscapeString(slug), l, l)
		} else {
			fmt.Fprintf(w, `<a href="/%s/-/browse?letter=%s">%s</a>`,
				html.EscapeString(slug), l, l)
		}
	}
	cls := ""
	if letter == "#" {
		cls = ` class="active"`
	}
	fmt.Fprintf(w, `<a href="/%s/-/browse?letter=%%23"%s>#</a>`, html.EscapeString(slug), cls)
	fmt.Fprint(w, `</div>`)

	if letter != "" {
		var entries []searchResult
		var letterCount int

		if letter == "#" {
			// Non-alpha: collect all C entries starting with a non-letter rune.
			for e := range ze.archive.EntriesByTitlePrefix('C', "") {
				t := e.Title()
				if t == "" {
					continue
				}
				ru, _ := utf8.DecodeRuneInString(t)
				if !unicode.IsLetter(ru) {
					entries = append(entries, searchResult{
						Path:  "/" + slug + "/" + e.Path(),
						Title: t,
					})
				}
			}
			letterCount = len(entries)
		} else {
			// Collect entries for both cases (e.g., "A" and "a") and merge.
			upper := strings.ToUpper(letter)
			lower := strings.ToLower(letter)
			letterCount = ze.archive.TitlePrefixCount('C', upper)
			if lower != upper {
				letterCount += ze.archive.TitlePrefixCount('C', lower)
			}
			for e := range ze.archive.EntriesByTitlePrefix('C', upper) {
				entries = append(entries, searchResult{
					Path:  "/" + slug + "/" + e.Path(),
					Title: e.Title(),
				})
				if len(entries) >= offset+limit {
					break
				}
			}
			if lower != upper && len(entries) < offset+limit {
				for e := range ze.archive.EntriesByTitlePrefix('C', lower) {
					entries = append(entries, searchResult{
						Path:  "/" + slug + "/" + e.Path(),
						Title: e.Title(),
					})
					if len(entries) >= offset+limit {
						break
					}
				}
			}
		}

		fmt.Fprintf(w, `<p class="letter-info">%s articles</p>`, commaInt(letterCount))
		fmt.Fprint(w, `<div class="entries">`)
		if letterCount == 0 || offset >= letterCount {
			fmt.Fprint(w, `<p style="color:#666;font-style:italic">No entries.</p>`)
		} else {
			end := offset + limit
			if end > len(entries) {
				end = len(entries)
			}
			for _, res := range entries[offset:end] {
				fmt.Fprintf(w, `<a href="%s">%s</a>`, html.EscapeString(res.Path), html.EscapeString(res.Title))
			}
		}
		fmt.Fprint(w, `</div>`)

		if letterCount > 0 {
			pageEnd := offset + limit
			if pageEnd > letterCount {
				pageEnd = letterCount
			}
			fmt.Fprint(w, `<div class="pager">`)
			if offset > 0 {
				prev := offset - limit
				if prev < 0 {
					prev = 0
				}
				fmt.Fprintf(w, `<a href="/%s/-/browse?letter=%s&offset=%d&limit=%d">← Previous</a>`,
					html.EscapeString(slug), html.EscapeString(letter), prev, limit)
			}
			fmt.Fprintf(w, `<span>%s–%s of %s</span>`,
				commaInt(offset+1), commaInt(pageEnd), commaInt(letterCount))
			if offset+limit < letterCount {
				fmt.Fprintf(w, `<a href="/%s/-/browse?letter=%s&offset=%d&limit=%d">Next →</a>`,
					html.EscapeString(slug), html.EscapeString(letter), offset+limit, limit)
			}
			fmt.Fprint(w, `</div>`)
		}
	}

	fmt.Fprintf(w, `<div class="nav"><a href="/">Library</a> · <a href="/%s/">Back to main page</a> · <a href="/%s/-/search">Search</a> · <a href="/%s/-/random">Random article</a></div>`,
		html.EscapeString(slug), html.EscapeString(slug), html.EscapeString(slug))
	fmt.Fprint(w, `</body></html>`)
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
		http.NotFound(w, r)
		return
	}

	// Root of a ZIM: serve main page or redirect to it
	if contentPath == "" {
		if !ze.archive.HasMainEntry() {
			http.Error(w, "no main page", http.StatusNotFound)
			return
		}
		main, err := ze.archive.MainEntry()
		if err != nil {
			log.Printf("error reading main entry for %s: %v", slug, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		resolved, err := main.Resolve()
		if err != nil {
			log.Printf("error resolving main entry for %s: %v", slug, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/"+slug+"/"+resolved.Path(), http.StatusFound)
		return
	}

	// Favicon shortcut
	if contentPath == "favicon.ico" {
		data, err := ze.archive.Illustration(48)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
		return
	}

	// Look up entry in C namespace
	entry, err := ze.archive.EntryByPath("C/" + contentPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Handle redirects within the ZIM
	if entry.IsRedirect() {
		resolved, err := entry.Resolve()
		if err != nil {
			log.Printf("error resolving redirect for %s/%s: %v", slug, contentPath, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/"+slug+"/"+resolved.Path(), http.StatusFound)
		return
	}

	// ETag / conditional request
	etag := makeETag(ze, entry.FullPath())
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Get content size and reader
	size, err := entry.ContentSize()
	if err != nil {
		log.Printf("error reading content for %s/%s: %v", slug, contentPath, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	reader, err := entry.ContentReader()
	if err != nil {
		log.Printf("error reading content for %s/%s: %v", slug, contentPath, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers. Append charset for text types since ZIM MIME types
	// typically omit it and browsers may guess wrong without it.
	mime := entry.MIMEType()
	if mime != "" {
		if strings.HasPrefix(mime, "text/") && !strings.Contains(mime, "charset") {
			mime += "; charset=utf-8"
		}
		w.Header().Set("Content-Type", mime)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)

	// For text/html content, inject a navigation header bar.
	if entry.MIMEType() == "text/html" {
		body, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("error reading html content for %s/%s: %v", slug, contentPath, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		bar := headerBarHTML(slug, ze.title, ze.archive)
		body = injectHeaderBar(body, []byte(bar))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write(body)
		return
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	io.Copy(w, reader)
}

// injectHeaderBar inserts the header bar HTML after the opening <body...> tag.
// If no <body> tag is found, it prepends the bar to the content.
func injectHeaderBar(body, bar []byte) []byte {
	// Case-insensitive search for <body
	lower := bytes.ToLower(body)
	idx := bytes.Index(lower, []byte("<body"))
	if idx == -1 {
		return append(bar, body...)
	}
	// Find the closing > of the <body ...> tag
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

// headerBarHTML returns a self-contained HTML+CSS navigation bar for injection into ZIM pages.
func headerBarHTML(slug, title string, a *zim.Archive) string {
	es := html.EscapeString(slug)
	et := html.EscapeString(title)

	var b strings.Builder
	b.WriteString(`<style>
#gzim-bar{position:sticky;top:0;z-index:999999;background:#f6f8fa;border-bottom:1px solid #d0d7de;padding:4px 12px;font:13px/1.4 system-ui,sans-serif;display:flex;align-items:center;gap:10px;flex-wrap:wrap;box-sizing:border-box}
#gzim-bar *{box-sizing:border-box;margin:0;padding:0}
#gzim-bar a{color:#0366d6;text-decoration:none}
#gzim-bar a:hover{text-decoration:underline}
#gzim-bar .gzim-title{font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:200px}
#gzim-bar .gzim-sep{color:#d0d7de}
#gzim-bar form{display:flex;gap:4px}
#gzim-bar input[type=text]{padding:2px 6px;border:1px solid #d0d7de;border-radius:3px;font-size:13px;width:160px}
#gzim-bar input[type=text]:focus{outline:none;border-color:#0366d6}
#gzim-bar .gzim-btn{padding:2px 8px;border:1px solid #d0d7de;border-radius:3px;background:#fff;font-size:13px;cursor:pointer;color:#0366d6;text-decoration:none;white-space:nowrap}
#gzim-bar .gzim-btn:hover{background:#f0f3f6;text-decoration:none}
#gzim-bar .gzim-az{display:flex;gap:2px;flex-wrap:wrap}
#gzim-bar .gzim-az a{padding:1px 4px;border-radius:2px;font-size:12px;font-weight:600}
#gzim-bar .gzim-az a:hover{background:#ddf4ff;text-decoration:none}
#gzim-bar .gzim-az span{padding:1px 4px;font-size:12px;font-weight:600;color:#ccc}
body{padding-top:0!important}
</style>`)

	b.WriteString(`<div id="gzim-bar">`)
	fmt.Fprintf(&b, `<a href="/" title="Library">📚</a>`)
	b.WriteString(`<span class="gzim-sep">|</span>`)
	fmt.Fprintf(&b, `<a class="gzim-title" href="/%s/" title="%s">%s</a>`, es, et, et)
	fmt.Fprintf(&b, `<form action="/%s/-/search" method="get"><input type="text" name="q" placeholder="Search…"><button class="gzim-btn" type="submit">Search</button></form>`, es)
	fmt.Fprintf(&b, `<a class="gzim-btn" href="/%s/-/random">Random</a>`, es)

	b.WriteString(`<span class="gzim-sep">|</span><span class="gzim-az">`)
	for c := byte('A'); c <= 'Z'; c++ {
		count := a.TitlePrefixCount('C', string(c)) +
			a.TitlePrefixCount('C', strings.ToLower(string(c)))
		if count > 0 {
			fmt.Fprintf(&b, `<a href="/%s/-/browse?letter=%c">%c</a>`, es, c, c)
		} else {
			fmt.Fprintf(&b, `<span>%c</span>`, c)
		}
	}
	b.WriteString(`</span>`)
	b.WriteString(`</div>`)

	return b.String()
}
