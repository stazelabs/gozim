package main

import (
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed static templates
var embeddedFS embed.FS

// staticFS is the sub-filesystem rooted at static/ for serving /_static/ files.
var staticFS fs.FS

var tmplFuncs = template.FuncMap{
	"commaInt":    commaInt,
	"formatBytes": formatBytes,
}

var (
	tmplError             *template.Template
	tmplIndex             *template.Template
	tmplSearch            *template.Template
	tmplBrowse            *template.Template
	tmplDocs              *template.Template
	tmplInfo              *template.Template
	tmplInfoNS            *template.Template
	tmplInfoMIME          *template.Template
	tmplInfoEntry         *template.Template
	tmplInfoClusterList   *template.Template
	tmplInfoClusterDetail *template.Template
	tmplServerInfo        *template.Template
	tmplBar               *template.Template
)

func init() {
	sub, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		panic("zimserve: failed to create static sub-FS: " + err.Error())
	}
	staticFS = sub

	mustParse := func(files ...string) *template.Template {
		t, err := template.New("").Funcs(tmplFuncs).ParseFS(embeddedFS, files...)
		if err != nil {
			panic("zimserve: failed to parse templates [" + strings.Join(files, ", ") + "]: " + err.Error())
		}
		return t
	}

	tmplError = mustParse("templates/error.html")
	layout := "templates/layout.html"
	tmplIndex = mustParse(layout, "templates/index.html")
	tmplSearch = mustParse(layout, "templates/search.html")
	tmplBrowse = mustParse(layout, "templates/browse.html")
	tmplDocs = mustParse(layout, "templates/docs.html")
	tmplInfo = mustParse(layout, "templates/info.html")
	tmplInfoNS = mustParse(layout, "templates/info_ns.html")
	tmplInfoMIME = mustParse(layout, "templates/info_mime.html")
	tmplInfoEntry = mustParse(layout, "templates/info_entry.html")
	tmplInfoClusterList = mustParse(layout, "templates/info_cluster_list.html")
	tmplInfoClusterDetail = mustParse(layout, "templates/info_cluster_detail.html")
	tmplServerInfo = mustParse(layout, "templates/server_info.html")
	tmplBar = mustParse("templates/bar.html")
}

// renderWith sets Content-Type and executes the "layout" template with data.
func renderWith(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}

// renderBarHTML renders the bar template to a string for injection into ZIM pages.
func renderBarHTML(slug, title string, letters []barLetterInfo) string {
	data := struct {
		Slug    string
		Title   string
		Letters []barLetterInfo
	}{slug, title, letters}
	var buf strings.Builder
	if err := tmplBar.ExecuteTemplate(&buf, "bar", data); err != nil {
		log.Printf("bar template error: %v", err)
		return ""
	}
	return buf.String()
}
