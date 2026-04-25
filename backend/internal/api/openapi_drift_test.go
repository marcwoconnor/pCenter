package api

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPINoDrift asserts that every HTTP route registered on any mux in
// router.go is either documented in openapi.yaml or explicitly allowlisted
// in testdata/openapi_drift_allowlist.txt (one "METHOD /path" per line).
//
// Why: #26 shipped a hand-authored spec. Without a guardrail, adding a route
// to router.go and forgetting to update the spec silently degrades the docs.
// This test closes that loop. #32 tracks shrinking the allowlist toward zero.
//
// Approach: regex over the router.go source (Option B from #35). The Go 1.22
// stdlib mux doesn't expose registered routes via public API, so the
// alternative (Option A) would require instrumenting 190+ HandleFunc calls.
// Textual extraction is one test file, no production changes.
func TestOpenAPINoDrift(t *testing.T) {
	registered := extractRegisteredRoutes(t)
	documented := extractDocumentedRoutes(t)
	allowlist := loadAllowlist(t)

	var undocumented []string
	for r := range registered {
		if documented[r] || allowlist[r] {
			continue
		}
		undocumented = append(undocumented, r)
	}
	sort.Strings(undocumented)

	if len(undocumented) > 0 {
		t.Errorf("%d route(s) registered in router.go but not documented in openapi.yaml "+
			"and not in testdata/openapi_drift_allowlist.txt:\n  %s\n\n"+
			"Fix options:\n"+
			"  (a) Add path entries to backend/internal/api/openapi.yaml (preferred).\n"+
			"  (b) Append the route(s) to testdata/openapi_drift_allowlist.txt if you're "+
			"shipping a new route under an as-yet-undocumented feature area (see #32).",
			len(undocumented), strings.Join(undocumented, "\n  "))
	}

	// Also flag dead entries: routes in the allowlist that no longer exist
	// in router.go. Keeps the allowlist from rotting.
	var stale []string
	for r := range allowlist {
		if !registered[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("%d entry(ies) in testdata/openapi_drift_allowlist.txt no longer correspond "+
			"to registered routes — remove them:\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}

// routeLineRe matches `<anything>Mux.HandleFunc("METHOD /path", ...)` or just
// `mux.HandleFunc(...)`. Group 1 = "METHOD /path".
var routeLineRe = regexp.MustCompile(`\b[A-Za-z_]+[Mm]ux\.HandleFunc\("([A-Z]+ [^"]+)"`)

func extractRegisteredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	out := make(map[string]bool)
	for _, m := range routeLineRe.FindAllSubmatch(src, -1) {
		out[string(m[1])] = true
	}
	if len(out) == 0 {
		t.Fatal("extracted zero routes from router.go — regex may be broken")
	}
	return out
}

func extractDocumentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	// openAPIYAML is the same []byte the production server embeds at startup
	// (see openapi.go). Parsing the embedded bytes means this test also
	// catches YAML parse errors before they hit runtime init.
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openAPIYAML, &spec); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	validMethods := map[string]bool{
		"get": true, "post": true, "put": true, "patch": true, "delete": true, "head": true, "options": true,
	}
	out := make(map[string]bool)
	for path, ops := range spec.Paths {
		for method := range ops {
			lower := strings.ToLower(method)
			if !validMethods[lower] {
				continue // skip "parameters", "summary", and other non-op keys
			}
			out[strings.ToUpper(method)+" "+path] = true
		}
	}
	return out
}

func loadAllowlist(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("testdata", "openapi_drift_allowlist.txt")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}
		}
		t.Fatalf("open allowlist: %v", err)
	}
	defer f.Close()

	out := make(map[string]bool)
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Each entry must be "METHOD /path" — same shape as registered routes.
		if !strings.Contains(line, " /") {
			t.Fatalf("%s:%d: malformed allowlist entry %q (expected \"METHOD /path\")",
				path, lineNo, line)
		}
		out[line] = true
	}
	if err := s.Err(); err != nil {
		t.Fatalf("scan allowlist: %v", err)
	}
	return out
}
