// Package apidocs serves the OpenAPI description of this API, and a Swagger UI
// page to read it in.
//
// The document is hand-written and embedded in the binary, so a deployed build
// always serves the description of itself rather than of whatever was last
// generated. `TestEveryRouteIsDocumented` walks the real router and fails if
// the two drift apart, which is the only thing that keeps a hand-written spec
// honest.
package apidocs

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var spec []byte

// Spec returns the raw OpenAPI document, for tests and tooling.
func Spec() []byte { return spec }

// swaggerUIVersion is pinned to a major version: patch releases of Swagger UI
// are safe to pick up, a major one could change the page out from under us.
const swaggerUIVersion = "5"

// Routes mounts the documentation. It is deliberately public — an API meant for
// third parties that hides its own description is not much use to them, and the
// document contains no secrets.
func Routes(r chi.Router) {
	r.Get("/openapi.yaml", handleSpec)
	r.Get("/docs", handleUI)
}

func handleSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	// Cheap to regenerate and pinned to the build, so a short cache is enough
	// to stop the UI refetching it on every navigation.
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(spec)
}

func handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The page loads Swagger UI from a CDN rather than vendoring ~3 MB of
	// assets into the repository and the image. It is the only part of this
	// service that needs the public internet to render.
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; "+
			"style-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; "+
			"img-src 'self' data: https://cdn.jsdelivr.net; "+
			"connect-src 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(uiPage)
}

var uiPage = []byte(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>x-mail API</title>
<link rel="icon" href="data:,">
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@` + swaggerUIVersion + `/swagger-ui.css">
<style>
  body { margin: 0; background: #fafafa; }
  .swagger-ui .topbar { display: none; }
  /* The spec's own description carries the orientation, so the generated
     header only adds noise above it. */
  .swagger-ui .info { margin: 2rem 0; }
</style>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@` + swaggerUIVersion + `/swagger-ui-bundle.js" crossorigin></script>
<script>
  window.ui = SwaggerUIBundle({
    url: 'openapi.yaml',
    dom_id: '#swagger-ui',
    deepLinking: true,
    // "Try it out" runs against this same origin, so a signed-in browser can
    // exercise the API with its own session cookie.
    withCredentials: true,
    persistAuthorization: true,
    defaultModelsExpandDepth: 0,
    // 'list' shows every operation name under its tag without expanding the
    // bodies: 'none' collapses the tags themselves, which hides the whole API
    // behind fourteen headings on first visit.
    docExpansion: 'list',
    tryItOutEnabled: true,
  });
</script>
</body>
</html>
`)
