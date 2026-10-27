package httpapi

import (
	"net/http"
	"os"
)

// swaggerPage loads Swagger UI from a pinned CDN build and points it at our spec. It is served
// from the API's own origin, so "Try it out" sends the session cookie and passes the origin
// check like the real web app.
const swaggerPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cashflow API docs</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>
window.ui = SwaggerUIBundle({ url: "/docs/openapi.yaml", dom_id: "#swagger-ui", withCredentials: true });
</script>
</body>
</html>`

// docsPage serves Swagger UI. Registered only when DocsSpecPath is set (development).
func (s *Server) docsPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write([]byte(swaggerPage)); err != nil {
		s.log.Error("writing docs page", "error", err)
	}
}

// docsSpec serves api/openapi.yaml from disk, so edits show up on reload.
func (s *Server) docsSpec(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.DocsSpecPath)
	if err != nil {
		s.log.ErrorContext(r.Context(), "reading OpenAPI spec", "path", s.DocsSpecPath, "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal", "Spesifikasi API tidak ditemukan.")
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(data); err != nil {
		s.log.Error("writing OpenAPI spec", "error", err)
	}
}
