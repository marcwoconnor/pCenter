package api

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openAPIYAML []byte

//go:embed swagger-ui/swagger-ui.css
var swaggerUICSS []byte

//go:embed swagger-ui/swagger-ui-bundle.js
var swaggerUIJS []byte

// openAPIJSON is the YAML spec converted once at startup.
var openAPIJSON []byte

func init() {
	var spec any
	if err := yaml.Unmarshal(openAPIYAML, &spec); err != nil {
		panic("openapi.yaml failed to parse: " + err.Error())
	}
	b, err := json.Marshal(spec)
	if err != nil {
		panic("openapi.yaml failed to re-encode as JSON: " + err.Error())
	}
	openAPIJSON = b
}

func serveOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(openAPIYAML)
}

func serveOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(openAPIJSON)
}

// swaggerUIHTML renders Swagger UI pointing at /api/openapi.yaml. Assets are
// served from /api/swagger-ui/* (embedded in the binary) so /api/docs works
// on air-gapped deploys with no outbound internet access. See
// `backend/internal/api/swagger-ui/README.md` for the pinned version + how
// to upgrade.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>pCenter API</title>
  <link rel="stylesheet" href="/api/swagger-ui/swagger-ui.css">
  <style>body { margin: 0; } #swagger-ui { max-width: 1400px; margin: 0 auto; }</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="/api/swagger-ui/swagger-ui-bundle.js"></script>
<script>
  window.ui = SwaggerUIBundle({
    url: '/api/openapi.yaml',
    dom_id: '#swagger-ui',
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis],
    layout: 'BaseLayout',
  });
</script>
</body>
</html>`

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUIHTML))
}

// Swagger UI asset handlers — static files, embedded at compile time, cached
// aggressively since they're versioned by the repo itself (change = redeploy).
func serveSwaggerUICSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Write(swaggerUICSS)
}

func serveSwaggerUIJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Write(swaggerUIJS)
}
