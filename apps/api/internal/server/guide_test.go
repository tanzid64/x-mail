package server_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The written guide is the documentation someone actually reads; the OpenAPI
// document is for tooling. Both are hand-maintained, so both drift, and a
// reference that quietly stops matching the server is worse than none.
//
// This test reaches across into the web app's source, which is unusual for a Go
// test. It is deliberate: the two halves of the repository describe one API,
// and this is the only place that has both the route table and a YAML parser
// already to hand.

// guideEndpoint matches an <Endpoint method="…" path="…"> in a docs page.
var guideEndpoint = regexp.MustCompile(`<Endpoint[^>]*?method="([A-Z]+)"\s+path="([^"]+)"`)

// notInTheGuide is the set the guide deliberately leaves out of its endpoint
// reference. Each needs a reason, and the reason is checked by this list
// existing rather than by the test silently ignoring absences.
var notInTheGuide = map[string]string{
	// Written up as a table rather than five near-identical entries.
	"GET /ping":     "covered in the health table",
	"GET /healthz":  "covered in the health table",
	"HEAD /healthz": "covered in the health table",
	"GET /readyz":   "covered in the health table",
	"HEAD /readyz":  "covered in the health table",

	// The guide is written to stand on its own, for a reader who never opens
	// Swagger. Pointing at it from inside would undercut that.
	"GET /docs":         "the guide does not reference Swagger",
	"GET /openapi.yaml": "the guide does not reference Swagger",
}

func guideDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "web", "src", "docs", "pages"))
	if err != nil {
		t.Fatalf("resolve guide directory: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("guide pages not present at %s: %v", dir, err)
	}
	return dir
}

// guideRoutes is every endpoint the written reference documents.
func guideRoutes(t *testing.T) map[string]bool {
	t.Helper()

	entries, err := filepath.Glob(filepath.Join(guideDir(t), "api-*.tsx"))
	if err != nil {
		t.Fatalf("list guide pages: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no api-*.tsx pages found; the reference has moved or been deleted")
	}

	routes := make(map[string]bool)
	for _, file := range entries {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, match := range guideEndpoint.FindAllStringSubmatch(string(source), -1) {
			routes[match[1]+" "+match[2]] = true
		}
	}
	return routes
}

func TestTheWrittenGuideDocumentsEveryEndpoint(t *testing.T) {
	spec := loadSpec(t)
	documented := guideRoutes(t)

	var missing []string
	for route := range specRoutes(spec) {
		// specRoutes keys are lowercase-method; the guide writes them uppercase.
		method, path, _ := strings.Cut(route, " ")
		key := strings.ToUpper(method) + " " + path

		if documented[key] || notInTheGuide[key] != "" {
			continue
		}
		missing = append(missing, key)
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d endpoint(s) exist but the written guide does not document them:\n  %s\n\n"+
			"Add an <Endpoint> to apps/web/src/docs/pages/api-*.tsx, or add it to "+
			"notInTheGuide with a reason.",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestTheWrittenGuideDocumentsNothingImaginary(t *testing.T) {
	spec := loadSpec(t)
	real := specRoutes(spec)

	var phantom []string
	for route := range guideRoutes(t) {
		method, path, _ := strings.Cut(route, " ")
		if !real[strings.ToLower(method)+" "+path] {
			phantom = append(phantom, route)
		}
	}
	sort.Strings(phantom)

	if len(phantom) > 0 {
		t.Errorf("%d endpoint(s) are in the written guide but do not exist:\n  %s",
			len(phantom), strings.Join(phantom, "\n  "))
	}
}

// An exclusion that is no longer needed is its own kind of drift: it means the
// next person to remove an endpoint gets no warning that the guide still
// mentions it.
func TestTheGuidesExclusionsAreAllStillReal(t *testing.T) {
	real := specRoutes(loadSpec(t))

	for route, reason := range notInTheGuide {
		method, path, _ := strings.Cut(route, " ")
		if !real[strings.ToLower(method)+" "+path] {
			t.Errorf("notInTheGuide lists %q (%s), but no such endpoint exists any more", route, reason)
		}
	}
}
