package main

import (
	"html/template"
	"log"
	"net/http"
)

func writeErrorPage(w http.ResponseWriter, status int, heading, detail string) {
	icon := template.HTML("⚠︎")
	if status == http.StatusNotFound {
		icon = "∅"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmplError.ExecuteTemplate(w, "error", errorPageData{
		Status:     status,
		StatusText: http.StatusText(status),
		Icon:       icon,
		Heading:    heading,
		Detail:     detail,
	}); err != nil {
		log.Printf("error template execute error: %v", err)
	}
}

func write404(w http.ResponseWriter) {
	writeErrorPage(w, http.StatusNotFound,
		"Page not found",
		"The page you requested doesn\u2019t exist or has been moved.")
}

func write500(w http.ResponseWriter) {
	writeErrorPage(w, http.StatusInternalServerError,
		"Something went wrong",
		"An internal error occurred. Please try again or return to the library.")
}

func writeBadRequest(w http.ResponseWriter, detail string) {
	writeErrorPage(w, http.StatusBadRequest, "Bad request", detail)
}
