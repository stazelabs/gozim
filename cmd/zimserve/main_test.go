package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
