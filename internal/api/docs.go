package api

import (
	_ "embed"
	"net/http"
)

//go:embed static/openapi.yaml
var openapiSpec []byte

//go:embed static/docs.html
var docsHTML []byte

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Write(openapiSpec)
}

func serveDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(docsHTML)
}
