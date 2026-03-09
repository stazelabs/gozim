package main

import (
	"bytes"
	_ "embed"
	"html"
	"html/template"
	"net/http"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

//go:embed zimserve.md
var zimserveMD string

// renderedDocs is the pre-rendered HTML body of zimserve.md, computed at init time.
var renderedDocs string

func init() {
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	var buf bytes.Buffer
	if err := md.Convert([]byte(zimserveMD), &buf); err != nil {
		renderedDocs = "<pre>" + html.EscapeString(zimserveMD) + "</pre>"
		return
	}
	renderedDocs = buf.String()
}

func handleDocs(w http.ResponseWriter, r *http.Request) {
	renderWith(w, tmplDocs, docsData{Content: template.HTML(renderedDocs)})
}
