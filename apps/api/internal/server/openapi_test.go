package server_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/dark/x-mail/api/internal/apidocs"
)

// A hand-written OpenAPI document is only worth having while it matches the
// server. These tests walk the real router and compare it against the spec in
// both directions, so adding a route without documenting it — or documenting
// one that does not exist — fails the build rather than quietly misleading an
// integrator.

type openAPI struct {
	OpenAPI string `yaml:"openapi"`
	Info    struct {
		Title   string `yaml:"title"`
		Version string `yaml:"version"`
	} `yaml:"info"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Tags []struct {
		Name string `yaml:"name"`
	} `yaml:"tags"`
	// A path item mixes operations with other keys — `parameters`, `servers` —
	// so it is decoded per key rather than straight into a map of operations.
	Paths      map[string]map[string]yaml.Node `yaml:"paths"`
	Components struct {
		Schemas         map[string]yaml.Node `yaml:"schemas"`
		SecuritySchemes map[string]yaml.Node `yaml:"securitySchemes"`
		Responses       map[string]yaml.Node `yaml:"responses"`
	} `yaml:"components"`
}

type operation struct {
	Summary     string               `yaml:"summary"`
	Description string               `yaml:"description"`
	Tags        []string             `yaml:"tags"`
	Responses   map[string]yaml.Node `yaml:"responses"`
	// Security is a pointer so "absent" (inherit the global default) can be
	// told apart from "[]" (explicitly public).
	Security *[]map[string][]string `yaml:"security"`
}

// httpMethods are the keys in a path item that describe an operation. Anything
// else there — parameters, servers, summary — is not one.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

func loadSpec(t *testing.T) *openAPI {
	t.Helper()
	var spec openAPI
	if err := yaml.Unmarshal(apidocs.Spec(), &spec); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}
	return &spec
}

// operations yields the operations of one path item, skipping the keys that
// describe the path rather than an operation on it.
func (s *openAPI) operations(t *testing.T, path string) map[string]operation {
	t.Helper()
	out := make(map[string]operation)
	for method, node := range s.Paths[path] {
		if !httpMethods[method] {
			continue
		}
		var op operation
		if err := node.Decode(&op); err != nil {
			t.Fatalf("%s %s: %v", strings.ToUpper(method), path, err)
		}
		out[method] = op
	}
	return out
}

// routeKey normalises a chi route into the form the spec uses: no trailing
// slash on an index route, and stripped of the /api/v1 prefix that the spec
// carries in `servers` instead.
func routeKey(method, route string) string {
	route = strings.TrimSuffix(route, "/*")
	if route != "/api/v1/" && len(route) > 1 {
		route = strings.TrimSuffix(route, "/")
	}
	if after, found := strings.CutPrefix(route, "/api/v1"); found {
		route = after
		if route == "" {
			route = "/"
		}
	}
	return strings.ToLower(method) + " " + route
}

// realRoutes is every operation the server actually serves.
func realRoutes(t *testing.T, handler http.Handler) map[string]bool {
	t.Helper()
	router, ok := handler.(chi.Router)
	if !ok {
		t.Fatalf("handler is %T, not a chi.Router", handler)
	}

	routes := make(map[string]bool)
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[routeKey(method, route)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	return routes
}

// specRoutes is every operation the document claims.
func specRoutes(spec *openAPI) map[string]bool {
	routes := make(map[string]bool)
	for path, item := range spec.Paths {
		for method := range item {
			if httpMethods[method] {
				routes[method+" "+path] = true
			}
		}
	}
	return routes
}

func TestEveryRouteIsDocumented(t *testing.T) {
	c := newTestServer(t)
	spec := loadSpec(t)

	real := realRoutes(t, c.handler)
	documented := specRoutes(spec)

	var undocumented []string
	for route := range real {
		if !documented[route] {
			undocumented = append(undocumented, route)
		}
	}
	sort.Strings(undocumented)

	if len(undocumented) > 0 {
		t.Errorf("%d route(s) exist but are not in openapi.yaml:\n  %s",
			len(undocumented), strings.Join(undocumented, "\n  "))
	}
}

func TestEveryDocumentedRouteExists(t *testing.T) {
	c := newTestServer(t)
	spec := loadSpec(t)

	real := realRoutes(t, c.handler)
	documented := specRoutes(spec)

	var phantom []string
	for route := range documented {
		if !real[route] {
			phantom = append(phantom, route)
		}
	}
	sort.Strings(phantom)

	if len(phantom) > 0 {
		t.Errorf("%d route(s) are documented but do not exist:\n  %s",
			len(phantom), strings.Join(phantom, "\n  "))
	}
}

func TestTheSpecIsWellFormedEnoughToBeUseful(t *testing.T) {
	spec := loadSpec(t)

	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		t.Errorf("openapi version %q, want 3.x", spec.OpenAPI)
	}
	if spec.Info.Title == "" || spec.Info.Version == "" {
		t.Error("info.title and info.version are both required")
	}
	if len(spec.Servers) == 0 {
		t.Error("no servers declared, so a generated client has no base URL")
	}

	// Both ways of authenticating must be described, or an integrator cannot
	// tell that a token is an option at all.
	for _, scheme := range []string{"sessionCookie", "apiToken"} {
		if _, ok := spec.Components.SecuritySchemes[scheme]; !ok {
			t.Errorf("security scheme %q is missing", scheme)
		}
	}

	declaredTags := make(map[string]bool)
	for _, tag := range spec.Tags {
		declaredTags[tag.Name] = true
	}

	for path := range spec.Paths {
		for method, op := range spec.operations(t, path) {
			where := fmt.Sprintf("%s %s", strings.ToUpper(method), path)

			if op.Summary == "" {
				t.Errorf("%s: no summary", where)
			}
			if len(op.Responses) == 0 {
				t.Errorf("%s: no responses documented", where)
			}
			// A generated client needs to know which tag — and therefore which
			// group — an operation belongs to.
			if len(op.Tags) == 0 {
				t.Errorf("%s: no tag", where)
			}
			for _, tag := range op.Tags {
				if !declaredTags[tag] {
					t.Errorf("%s: tag %q is not declared in the top-level tags list", where, tag)
				}
			}
		}
	}
}

// Authenticated operations must document a 401, or a client generated from the
// spec has no branch for the credential expiring.
func TestAuthenticatedOperationsDocumentUnauthorized(t *testing.T) {
	spec := loadSpec(t)

	for path := range spec.Paths {
		for method, op := range spec.operations(t, path) {
			// An explicit empty `security: []` marks a public endpoint.
			if op.Security != nil && len(*op.Security) == 0 {
				continue
			}
			if _, ok := op.Responses["401"]; !ok {
				t.Errorf("%s %s requires a credential but documents no 401",
					strings.ToUpper(method), path)
			}
		}
	}
}

// The two session-only endpoints must say so in the document, since that is the
// one place an integrator finds out before their token gets a 403.
func TestSessionOnlyEndpointsAreMarkedInTheSpec(t *testing.T) {
	spec := loadSpec(t)

	sessionOnly := []struct{ method, path string }{
		{"post", "/me/password"},
		{"get", "/api-tokens"},
		{"post", "/api-tokens"},
		{"delete", "/api-tokens/{tokenID}"},
	}

	for _, want := range sessionOnly {
		op, ok := spec.operations(t, want.path)[want.method]
		if !ok {
			t.Errorf("%s %s is missing from the spec", strings.ToUpper(want.method), want.path)
			continue
		}
		where := fmt.Sprintf("%s %s", strings.ToUpper(want.method), want.path)

		if op.Security == nil {
			t.Errorf("%s inherits the global security, which would tell an integrator a token works here", where)
			continue
		}
		for _, requirement := range *op.Security {
			if _, hasToken := requirement["apiToken"]; hasToken {
				t.Errorf("%s lists apiToken, but the server refuses tokens there", where)
			}
		}
		if _, ok := op.Responses["403"]; !ok {
			t.Errorf("%s does not document the 403 a token gets", where)
		}
	}
}
