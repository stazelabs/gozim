package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
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
	uuidHex     string // hex-encoded UUID for ETag generation
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
	mux.HandleFunc("/_random", lib.handleRandomAll)
	mux.HandleFunc("/_search", lib.handleSearchAll)
	mux.HandleFunc("/{slug}/_search", lib.handleSearchJSON)
	mux.HandleFunc("/{slug}/-/search", lib.handleSearchPage)
	mux.HandleFunc("/{slug}/-/random", lib.handleRandom)
	mux.HandleFunc("/{slug}/-/browse", lib.handleBrowse)
	mux.HandleFunc("/{slug}/-/info", lib.handleInfo)
	mux.HandleFunc("/{slug}/-/info/ns", lib.handleInfoNamespace)
	mux.HandleFunc("/{slug}/-/info/mime", lib.handleInfoMIME)
	mux.HandleFunc("/{slug}/-/info/entry", lib.handleInfoEntry)
	mux.HandleFunc("/{slug}/{path...}", lib.handleContent)

	srv := &http.Server{
		Addr:         addr,
		Handler:      securityHeaders(methodCheck(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	done := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-done
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

		uuid := a.UUID()
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
			uuidHex:     hex.EncodeToString(uuid[:]),
		}
		lib.slugs = append(lib.slugs, slug)
	}

	if len(lib.slugs) == 0 {
		return nil, errors.New("no valid ZIM files found")
	}
	sort.Slice(lib.slugs, func(i, j int) bool {
		return strings.ToLower(lib.archives[lib.slugs[i]].title) < strings.ToLower(lib.archives[lib.slugs[j]].title)
	})
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

// makeETag generates an ETag for a content entry from the archive UUID and path.
func makeETag(ze *zimEntry, entryPath string) string {
	h := md5.New()
	h.Write([]byte(ze.uuidHex))
	h.Write([]byte(entryPath))
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`
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
