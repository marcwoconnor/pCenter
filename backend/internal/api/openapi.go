package api

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var openAPIYAML []byte

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

// swaggerUIHTML renders Swagger UI pointing at /api/openapi.yaml.
// NOTE: assets are loaded from jsdelivr; for air-gapped deployments, vendor them locally.
// Tracked as a follow-up issue.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>pCenter API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>body { margin: 0; } #swagger-ui { max-width: 1400px; margin: 0 auto; }</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
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
