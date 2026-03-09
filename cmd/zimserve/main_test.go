package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func testZIM(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "small.zim")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		t.Skipf("test ZIM not found: %s", p)
	}
	return p
}

func TestMakeSlug(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"wikipedia_en_all_2024-01.zim", "wikipedia_en_all"},
		{"small.zim", "small"},
		{"wiki_2024-01-15.zim", "wiki"},
		{"test.zim", "test"},
	}
	for _, tt := range tests {
		got := makeSlug(tt.path)
		if got != tt.want {
			t.Errorf("makeSlug(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLoadLibrary(t *testing.T) {
	path := testZIM(t)
	lib, err := loadLibrary([]string{path}, 1, 16)
	if err != nil {
		t.Fatalf("loadLibrary failed: %v", err)
	}
	if len(lib.slugs) != 1 {
		t.Fatalf("expected 1 slug, got %d", len(lib.slugs))
	}
	for _, e := range lib.archives {
		e.archive.Close()
	}
}

func testLibrary(t *testing.T) *library {
	t.Helper()
	path := testZIM(t)
	lib, err := loadLibrary([]string{path}, 1, 16)
	if err != nil {
		t.Fatalf("loadLibrary failed: %v", err)
	}
	t.Cleanup(func() {
		for _, e := range lib.archives {
			e.archive.Close()
		}
	})
	return lib
}

func TestHandleRootSingleZIM(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	lib.handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "search-input") {
		t.Error("expected search box in index page")
	}
}

func TestHandleSearch(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/_search?q=Test", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleSearchJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "title") {
		t.Errorf("expected JSON results with title field, got: %s", body)
	}
}

func TestHandleSearchEmpty(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/_search?q=", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleSearchJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "[]" {
		t.Errorf("expected empty array, got: %s", rec.Body.String())
	}
}

func TestHandleContentNotFound(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/page", nil)
	req.SetPathValue("slug", "nonexistent")
	req.SetPathValue("path", "page")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleContentMainPage(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	// Should redirect to main page
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
}

func TestMethodCheck(t *testing.T) {
	handler := methodCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// POST should be rejected
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", rec.Code)
	}

	// GET should pass through
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options header")
	}
	if rec.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("missing X-Frame-Options header")
	}
}

// --- Item 11: HTML injection tests ---

func TestInjectHeaderBarWithBody(t *testing.T) {
	body := []byte(`<html><head></head><body><p>Hello</p></body></html>`)
	bar := []byte(`<div id="bar">NAV</div>`)
	result := injectHeaderBar(body, bar)
	s := string(result)

	// Bar should appear right after <body>
	if !strings.Contains(s, `<body><div id="bar">NAV</div><p>Hello</p>`) {
		t.Errorf("bar not injected after <body>: %s", s)
	}
}

func TestInjectHeaderBarWithBodyAttrs(t *testing.T) {
	body := []byte(`<html><BODY class="main" id="top"><p>Content</p></BODY></html>`)
	bar := []byte(`<nav>BAR</nav>`)
	result := injectHeaderBar(body, bar)
	s := string(result)

	// Should inject after the closing > of <BODY ...>
	if !strings.Contains(s, `id="top"><nav>BAR</nav><p>Content</p>`) {
		t.Errorf("bar not injected after <BODY> with attrs: %s", s)
	}
}

func TestInjectHeaderBarCaseInsensitive(t *testing.T) {
	body := []byte(`<html><Body><p>Mixed case</p></Body></html>`)
	bar := []byte(`<div>BAR</div>`)
	result := injectHeaderBar(body, bar)

	if !strings.Contains(string(result), `<Body><div>BAR</div><p>Mixed case</p>`) {
		t.Errorf("case-insensitive injection failed: %s", string(result))
	}
}

func TestInjectHeaderBarNoBody(t *testing.T) {
	body := []byte(`<html><p>No body tag</p></html>`)
	bar := []byte(`<div>BAR</div>`)
	result := injectHeaderBar(body, bar)

	// Should prepend bar to content
	if !strings.HasPrefix(string(result), `<div>BAR</div><html>`) {
		t.Errorf("bar not prepended when no <body>: %s", string(result))
	}
}

func TestInjectHeaderBarEmpty(t *testing.T) {
	result := injectHeaderBar([]byte{}, []byte(`<nav>BAR</nav>`))
	if string(result) != `<nav>BAR</nav>` {
		t.Errorf("empty body injection: %s", string(result))
	}
}

func TestInjectFooterBarWithBody(t *testing.T) {
	body := []byte(`<html><body><p>Content</p></body></html>`)
	bar := []byte(`<footer>FOOT</footer>`)
	result := injectFooterBar(body, bar)
	s := string(result)

	// Bar should appear before </body>
	if !strings.Contains(s, `<p>Content</p><footer>FOOT</footer></body>`) {
		t.Errorf("footer not injected before </body>: %s", s)
	}
}

func TestInjectFooterBarCaseInsensitive(t *testing.T) {
	body := []byte(`<html><body><p>Hi</p></BODY></html>`)
	bar := []byte(`<footer>F</footer>`)
	result := injectFooterBar(body, bar)

	if !strings.Contains(string(result), `<p>Hi</p><footer>F</footer></BODY>`) {
		t.Errorf("case-insensitive footer injection failed: %s", string(result))
	}
}

func TestInjectFooterBarNoBody(t *testing.T) {
	body := []byte(`<html><p>No closing body</p></html>`)
	bar := []byte(`<footer>F</footer>`)
	result := injectFooterBar(body, bar)

	// Should append
	if !strings.HasSuffix(string(result), `</html><footer>F</footer>`) {
		t.Errorf("footer not appended when no </body>: %s", string(result))
	}
}

// --- Item 12: Search and ETag tests ---

func TestSearchPrefixes(t *testing.T) {
	prefixes := searchPrefixes("test")
	// Should include original, Title case, and lowercase
	has := func(s string) bool {
		return slices.Contains(prefixes, s)
	}
	if !has("test") {
		t.Error("missing original prefix 'test'")
	}
	if !has("Test") {
		t.Error("missing title-case prefix 'Test'")
	}
}

func TestSearchPrefixesUnicode(t *testing.T) {
	prefixes := searchPrefixes("über")
	has := func(s string) bool {
		return slices.Contains(prefixes, s)
	}
	if !has("über") {
		t.Error("missing original 'über'")
	}
	if !has("Über") {
		t.Error("missing title-case 'Über'")
	}
}

func TestSearchPrefixesEmpty(t *testing.T) {
	prefixes := searchPrefixes("")
	if len(prefixes) != 1 || prefixes[0] != "" {
		t.Errorf("expected [\"\"], got %v", prefixes)
	}
}

func TestSearchPrefixesDedup(t *testing.T) {
	// "Test" — ToUpper('T') == 'T', so Title case == original
	prefixes := searchPrefixes("Test")
	seen := make(map[string]int)
	for _, p := range prefixes {
		seen[p]++
		if seen[p] > 1 {
			t.Errorf("duplicate prefix %q", p)
		}
	}
}

func TestMakeETag(t *testing.T) {
	ze := &zimEntry{uuidHex: "abcdef1234567890abcdef1234567890"}
	etag := makeETag(ze, "C/main.html")

	// Should be quoted and hex
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Errorf("ETag not quoted: %s", etag)
	}
	// Should be deterministic
	etag2 := makeETag(ze, "C/main.html")
	if etag != etag2 {
		t.Error("ETag not deterministic")
	}
	// Different path should give different ETag
	etag3 := makeETag(ze, "C/other.html")
	if etag == etag3 {
		t.Error("different paths should give different ETags")
	}
}

func TestHandleContentETag(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	// First request — should get content + ETag header
	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/main.html", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "main.html")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header")
	}

	// Second request with If-None-Match — should get 304
	req2 := httptest.NewRequest(http.MethodGet, "/"+slug+"/main.html", nil)
	req2.SetPathValue("slug", slug)
	req2.SetPathValue("path", "main.html")
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	lib.handleContent(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Errorf("expected 304 with matching ETag, got %d", rec2.Code)
	}
}

func TestHandleContentHTMLInjection(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/main.html", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "main.html")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Header bar should be injected
	if !strings.Contains(body, "gzim-bar") {
		t.Error("header bar not injected into HTML content")
	}
	// Footer bar should be injected
	if !strings.Contains(body, "gzim-footer") {
		t.Error("footer bar not injected into HTML content")
	}
}

func TestHandleContentRedirect(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	// small.zim has W/mainPage as a redirect
	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/main.html", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "main.html")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	// main.html is not a redirect itself, but the root is — test root redirect
	reqRoot := httptest.NewRequest(http.MethodGet, "/"+slug+"/", nil)
	reqRoot.SetPathValue("slug", slug)
	reqRoot.SetPathValue("path", "")
	recRoot := httptest.NewRecorder()
	lib.handleContent(recRoot, reqRoot)

	if recRoot.Code != http.StatusFound {
		t.Fatalf("expected 302 for root, got %d", recRoot.Code)
	}
	loc := recRoot.Header().Get("Location")
	if !strings.Contains(loc, slug) {
		t.Errorf("redirect location should contain slug: %s", loc)
	}
}

// --- Item 13: commaInt, search deduplication, other handlers ---

func TestCommaInt(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{1000000, "1,000,000"},
		{1234567890, "1,234,567,890"},
		{-1, "-1"},
		{-1000, "-1,000"},
	}
	for _, tt := range tests {
		got := commaInt(tt.n)
		if got != tt.want {
			t.Errorf("commaInt(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSearchArchiveDeduplication(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]
	ze := lib.archives[slug]

	// Search with a prefix that will match — results should not have duplicates
	var results []searchResult
	searchArchive(ze.archive, slug, "Test", &results, 100)

	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.Path] {
			t.Errorf("duplicate result path: %s", r.Path)
		}
		seen[r.Path] = true
	}
}

func TestSearchArchiveLimit(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]
	ze := lib.archives[slug]

	var results []searchResult
	searchArchive(ze.archive, slug, "T", &results, 1)

	if len(results) > 1 {
		t.Errorf("expected at most 1 result with limit=1, got %d", len(results))
	}
}

func TestHandleSearchAllJSON(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/_search?q=Test", nil)
	rec := httptest.NewRecorder()
	lib.handleSearchAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var results []searchResult
	if err := json.NewDecoder(rec.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	// Should find at least "Test ZIM file"
	if len(results) == 0 {
		t.Error("expected at least one search result")
	}
}

func TestHandleSearchAllEmpty(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/_search?q=", nil)
	rec := httptest.NewRecorder()
	lib.handleSearchAll(rec, req)

	if rec.Body.String() != "[]" {
		t.Errorf("expected empty array for empty query, got: %s", rec.Body.String())
	}
}

func TestHandleSearchPageHTML(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/-/search?q=Test", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleSearchPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "result") {
		t.Error("expected results in search page HTML")
	}
}

func TestHandleSearchPageJSON(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/-/search?q=Test&format=json", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleSearchPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestHandleSearchPageLimit(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/-/search?q=Test&limit=1&format=json", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleSearchPage(rec, req)

	var results []searchResult
	json.NewDecoder(rec.Body).Decode(&results)
	if len(results) > 1 {
		t.Errorf("expected at most 1 result with limit=1, got %d", len(results))
	}
}

func TestHandleBrowse(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/-/browse", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleBrowse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Should contain letter navigation
	for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		if !strings.Contains(body, string(letter)) {
			t.Errorf("missing letter %c in browse page", letter)
		}
	}
}

func TestHandleBrowseWithLetter(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/-/browse?letter=T", nil)
	req.SetPathValue("slug", slug)
	rec := httptest.NewRecorder()
	lib.handleBrowse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Test ZIM file") {
		t.Error("expected 'Test ZIM file' in browse results for letter T")
	}
}

func TestHandleBrowseNonExistentSlug(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/-/browse", nil)
	req.SetPathValue("slug", "nonexistent")
	rec := httptest.NewRecorder()
	lib.handleBrowse(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleRandomNonExistentSlug(t *testing.T) {
	lib := testLibrary(t)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/-/random", nil)
	req.SetPathValue("slug", "nonexistent")
	rec := httptest.NewRecorder()
	lib.handleRandom(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleContentFavicon(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/favicon.ico", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "favicon.ico")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	// small.zim has an illustration — should serve it
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for favicon, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("expected image/png, got %s", ct)
	}
}

func TestHandleContentNonHTMLNoInjection(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]

	// favicon.png is a non-HTML entry
	req := httptest.NewRequest(http.MethodGet, "/"+slug+"/favicon.png", nil)
	req.SetPathValue("slug", slug)
	req.SetPathValue("path", "favicon.png")
	rec := httptest.NewRecorder()
	lib.handleContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	// Should NOT have header bar injected into non-HTML
	if strings.Contains(rec.Body.String(), "gzim-bar") {
		t.Error("header bar should not be injected into non-HTML content")
	}
}

func TestHeaderBarHTML(t *testing.T) {
	lib := testLibrary(t)
	slug := lib.slugs[0]
	ze := lib.archives[slug]

	bar := headerBarHTML(slug, ze.title, ze.archive)

	if !strings.Contains(bar, "gzim-bar") {
		t.Error("header bar missing gzim-bar div")
	}
	if !strings.Contains(bar, "Search") {
		t.Error("header bar missing search form")
	}
	if !strings.Contains(bar, "Random") {
		t.Error("header bar missing random link")
	}
	// Should contain A-Z letter navigation
	if !strings.Contains(bar, `browse?letter=T`) {
		t.Error("header bar missing letter navigation")
	}
}

func TestFooterBarHTML(t *testing.T) {
	footer := footerBarHTML()
	if !strings.Contains(footer, "gzim-footer") {
		t.Error("footer missing gzim-footer div")
	}
	if !strings.Contains(footer, "zimserve") {
		t.Error("footer missing zimserve link")
	}
}

func TestMakeSlugEdgeCases(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/path/to/wiki.zim", "wiki"},
		{"no_extension", "no_extension"},
		{"nodash.zim", "nodash"},
		// Only strips trailing parts that are >= 4 chars and start with digit
		{"wiki_2024-01-15.zim", "wiki"},
		{"all_dates_2024_01_15.zim", "all_dates_2024_01_15"}, // "15" is only 2 chars
		{"a_1.zim", "a_1"},                                   // "1" is only 1 char
		{"just_numbers_1234.zim", "just_numbers"},            // "1234" is 4 chars
	}
	for _, tt := range tests {
		got := makeSlug(tt.path)
		if got != tt.want {
			t.Errorf("makeSlug(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
