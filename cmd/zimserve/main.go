package main

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/stazelabs/gozim/zim"
)

func main() {
	var addr string
	var cacheSize int
	var dirs []string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "zimserve [file.zim ...] [--dir <dir>]",
		Short: "Serve ZIM file content over HTTP",
		Long: `zimserve is an HTTP server for browsing ZIM file content.

Serves one or more ZIM files at http://localhost:8080 (by default).
Each ZIM is accessible under a URL slug derived from its filename.
If only one ZIM is loaded, the root URL redirects to its main page.

ZIM files may be specified as positional arguments, via --dir, or both.`,
		Args: func(cmd *cobra.Command, args []string) error {
			d, _ := cmd.Flags().GetStringArray("dir")
			if len(args) == 0 && len(d) == 0 {
				return errors.New("at least one ZIM file or --dir required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(args, dirs, recursive, addr, cacheSize)
		},
	}

	cmd.Flags().StringVarP(&addr, "addr", "a", ":8080", "listen address (host:port)")
	cmd.Flags().IntVarP(&cacheSize, "cache", "c", 64, "cluster cache size per ZIM file")
	cmd.Flags().StringArrayVarP(&dirs, "dir", "d", nil, "directory of ZIM files to serve (repeatable)")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "scan --dir directories recursively")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type library struct {
	archives map[string]*zimEntry // slug -> entry
	slugs    []string             // ordered list of slugs
}

type zimEntry struct {
	archive     *zim.Archive
	slug        string
	filename    string
	title       string
	language    string
	description string
	date        string
	creator     string
	flavour     string
}

func serve(paths []string, dirs []string, recursive bool, addr string, cacheSize int) error {
	dirPaths := collectZIMPaths(dirs, recursive)
	allPaths := append(paths, dirPaths...)
	lib, err := loadLibrary(allPaths, len(paths), cacheSize)
	if err != nil {
		return err
	}
	defer func() {
		for _, entry := range lib.archives {
			entry.archive.Close()
		}
	}()

	for _, slug := range lib.slugs {
		e := lib.archives[slug]
		log.Printf("loaded: /%s/ — %s (%d entries)", slug, e.title, e.archive.EntryCount())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", lib.handleRoot)
	mux.HandleFunc("/{slug}/{path...}", lib.handleContent)

	srv := &http.Server{
		Addr:         addr,
		Handler:      securityHeaders(methodCheck(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("listening on %s", addr)
	return srv.ListenAndServe()
}

// collectZIMPaths scans dirs for .zim files. Non-recursive by default;
// recursive mode uses filepath.WalkDir and does not follow directory symlinks.
// Results are deduplicated and sorted for deterministic slug assignment.
func collectZIMPaths(dirs []string, recursive bool) []string {
	seen := make(map[string]bool)
	var paths []string
	for _, dir := range dirs {
		if recursive {
			filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error { //nolint:errcheck
				if err != nil {
					log.Printf("warning: skipping %s: %v", path, err)
					return nil
				}
				// Don't follow symlinked directories to avoid cycles.
				if d.Type()&fs.ModeSymlink != 0 {
					info, err := os.Stat(path)
					if err == nil && info.IsDir() {
						return filepath.SkipDir
					}
				}
				if !d.IsDir() && strings.HasSuffix(path, ".zim") {
					if abs, err := filepath.Abs(path); err == nil && !seen[abs] {
						seen[abs] = true
						paths = append(paths, abs)
					}
				}
				return nil
			})
		} else {
			entries, err := os.ReadDir(dir)
			if err != nil {
				log.Printf("warning: cannot read directory %s: %v", dir, err)
				continue
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".zim") {
					if abs, err := filepath.Abs(filepath.Join(dir, e.Name())); err == nil && !seen[abs] {
						seen[abs] = true
						paths = append(paths, abs)
					}
				}
			}
		}
	}
	sort.Strings(paths)
	return paths
}

func loadLibrary(paths []string, hardFailCount int, cacheSize int) (*library, error) {
	lib := &library{
		archives: make(map[string]*zimEntry),
	}

	for i, path := range paths {
		a, err := zim.OpenWithOptions(path, zim.WithCacheSize(cacheSize))
		if err != nil {
			if i < hardFailCount {
				return nil, fmt.Errorf("opening %s: %w", path, err)
			}
			log.Printf("warning: skipping %s: %v", path, err)
			continue
		}

		slug := makeSlug(path)
		// Handle duplicate slugs
		base := slug
		for i := 2; lib.archives[slug] != nil; i++ {
			slug = fmt.Sprintf("%s_%d", base, i)
		}

		title, _ := a.Metadata("Title")
		if title == "" {
			title = slug
		}
		lang, _ := a.Metadata("Language")
		desc, _ := a.Metadata("Description")
		date, _ := a.Metadata("Date")
		creator, _ := a.Metadata("Creator")
		flavour, _ := a.Metadata("Flavour")

		lib.archives[slug] = &zimEntry{
			archive:     a,
			slug:        slug,
			filename:    filepath.Base(path),
			title:       title,
			language:    lang,
			description: desc,
			date:        date,
			creator:     creator,
			flavour:     flavour,
		}
		lib.slugs = append(lib.slugs, slug)
	}

	if len(lib.slugs) == 0 {
		return nil, errors.New("no valid ZIM files found")
	}
	return lib, nil
}

// makeSlug derives a URL-friendly slug from a ZIM filename.
// "wikipedia_en_all_2024-01.zim" -> "wikipedia_en_all"
func makeSlug(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".zim")
	// Strip date suffix (e.g., "_2024-01")
	parts := strings.Split(name, "_")
	for len(parts) > 1 {
		last := parts[len(parts)-1]
		if len(last) >= 4 && last[0] >= '0' && last[0] <= '9' {
			parts = parts[:len(parts)-1]
		} else {
			break
		}
	}
	return strings.Join(parts, "_")
}

// securityHeaders adds OWASP-recommended response headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// methodCheck rejects any method other than GET and HEAD.
func methodCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (lib *library) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Single ZIM: redirect to its main page
	if len(lib.slugs) == 1 {
		slug := lib.slugs[0]
		http.Redirect(w, r, "/"+slug+"/", http.StatusFound)
		return
	}

	// Multiple ZIMs: show library index
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
	fmt.Fprint(w, `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>zimserve</title>
<style>
body { font-family: system-ui, sans-serif; max-width: 1000px; margin: 40px auto; padding: 0 20px; }
h1 { border-bottom: 1px solid #ddd; padding-bottom: 10px; }
table { width: 100%; border-collapse: collapse; }
th { text-align: left; padding: 8px 10px; border-bottom: 2px solid #ddd; cursor: pointer; user-select: none; white-space: nowrap; }
th:hover { background: #f6f8fa; }
th.sorted { color: #0366d6; }
td { padding: 8px 10px; border-bottom: 1px solid #eee; vertical-align: top; }
td.num { text-align: right; white-space: nowrap; }
th.num { text-align: right; }
a { text-decoration: none; color: #0366d6; }
a:hover { text-decoration: underline; }
.sub { color: #666; font-size: 0.82em; margin-top: 2px; }
.arrow { font-size: 0.75em; margin-left: 4px; }
</style></head><body>
<h1>Library</h1>
<table><thead><tr>
<th data-col="0">Title<span class="arrow"></span></th>
<th data-col="1">File<span class="arrow"></span></th>
<th data-col="2">Date<span class="arrow"></span></th>
<th data-col="3" class="num">Entries<span class="arrow"></span></th>
</tr></thead><tbody>`)
	for _, slug := range lib.slugs {
		e := lib.archives[slug]

		// Title cell: link + optional description subtitle
		titleCell := fmt.Sprintf(`<a href="/%s/">%s</a>`, html.EscapeString(slug), html.EscapeString(e.title))
		if e.description != "" {
			titleCell += fmt.Sprintf(`<div class="sub">%s</div>`, html.EscapeString(e.description))
		}

		// File cell: filename + language + optional creator/flavour subtitle
		fileCell := html.EscapeString(e.filename)
		if e.language != "" {
			fileCell += fmt.Sprintf(` <span class="sub" style="display:inline">[%s]</span>`, html.EscapeString(e.language))
		}
		var metaParts []string
		if e.creator != "" {
			metaParts = append(metaParts, html.EscapeString(e.creator))
		}
		if e.flavour != "" {
			metaParts = append(metaParts, html.EscapeString(e.flavour))
		}
		if len(metaParts) > 0 {
			fileCell += fmt.Sprintf(`<div class="sub">%s</div>`, strings.Join(metaParts, " · "))
		}

		// Date cell
		dateVal := e.date
		dateDisplay := e.date
		if dateDisplay == "" {
			dateVal = ""
			dateDisplay = "—"
		}

		fmt.Fprintf(w, "<tr><td data-val=%q>%s</td><td data-val=%q>%s</td><td data-val=%q>%s</td><td data-val=%q class=\"num\">%d</td></tr>",
			e.title, titleCell,
			e.filename, fileCell,
			dateVal, html.EscapeString(dateDisplay),
			fmt.Sprintf("%d", e.archive.EntryCount()), e.archive.EntryCount())
	}
	fmt.Fprint(w, `</tbody></table>
<script>
(function(){
  var col = 0, asc = true;
  var ths = document.querySelectorAll('th[data-col]');
  var tbody = document.querySelector('tbody');
  function sort(c, a) {
    col = c; asc = a;
    ths.forEach(function(th, i) {
      var arrow = th.querySelector('.arrow');
      arrow.textContent = i === c ? (a ? ' \u25b2' : ' \u25bc') : '';
      th.classList.toggle('sorted', i === c);
    });
    var rows = Array.from(tbody.rows);
    rows.sort(function(ra, rb) {
      var av = ra.cells[c].dataset.val;
      var bv = rb.cells[c].dataset.val;
      var cmp = c === 3 ? +av - +bv : av.toLowerCase().localeCompare(bv.toLowerCase());
      return a ? cmp : -cmp;
    });
    rows.forEach(function(r){ tbody.appendChild(r); });
  }
  ths.forEach(function(th, i){
    th.addEventListener('click', function(){ sort(i, col === i ? !asc : true); });
  });
  sort(0, true);
})();
</script>
</body></html>`)
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

	// Read content
	data, err := entry.ReadContent()
	if err != nil {
		log.Printf("error reading content for %s/%s: %v", slug, contentPath, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Set Content-Type from MIME type
	mime := entry.MIMEType()
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	w.Write(data)
}
