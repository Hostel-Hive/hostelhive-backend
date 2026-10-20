package apidocs

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func parsed(t *testing.T) object {
	t.Helper()
	var spec object
	if err := json.Unmarshal(JSON(), &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

// Scan actual registration calls, not a second manually maintained route list.
func TestEveryBackendRouteHasCorrectModuleDocumentation(t *testing.T) {
	spec := parsed(t)
	paths := spec["paths"].(map[string]any)
	tags := map[string]string{}
	for _, v := range spec["tags"].([]any) {
		tag := v.(map[string]any)
		tags[tag["x-module"].(string)] = tag["name"].(string)
	}
	registered := map[string]bool{}
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if !(strings.HasSuffix(path, "_routes.go") || filepath.ToSlash(path) == "../app/router.go") {
			return nil
		}
		module := "app"
		if strings.HasSuffix(path, "_routes.go") {
			module = filepath.Base(filepath.Dir(path))
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Handle" && selector.Sel.Name != "HandleFunc") {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("unresolved route in %s", path)
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Error(err)
				return true
			}
			method, url, ok := strings.Cut(pattern, " ")
			if !ok {
				t.Errorf("methodless route %s", pattern)
				return true
			}
			registered[pattern] = true
			item, ok := paths[url].(map[string]any)
			if !ok {
				t.Errorf("missing path: %s", pattern)
				return true
			}
			op, ok := item[strings.ToLower(method)].(map[string]any)
			if !ok {
				t.Errorf("missing operation: %s", pattern)
				return true
			}
			opTags, ok := op["tags"].([]any)
			if !ok || len(opTags) != 1 || opTags[0] != tags[module] {
				t.Errorf("wrong module tag: %s (%s)", pattern, module)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	count := 0
	for path, v := range paths {
		for method, raw := range v.(map[string]any) {
			pattern := strings.ToUpper(method) + " " + path
			if !registered[pattern] {
				t.Errorf("documented route is not registered: %s", pattern)
			}
			op := raw.(map[string]any)
			id, _ := op["operationId"].(string)
			if id == "" || ids[id] {
				t.Errorf("missing/duplicate operationId %s", pattern)
			}
			ids[id] = true
			if op["summary"] == nil || op["description"] == nil || op["responses"] == nil {
				t.Errorf("incomplete operation %s", pattern)
			}
			params, _ := op["parameters"].([]any)
			names := map[string]bool{}
			for _, v := range params {
				p := v.(map[string]any)
				if p["in"] == "path" {
					if p["required"] != true {
						t.Errorf("optional path parameter %s", pattern)
					}
					names[p["name"].(string)] = true
				}
			}
			for _, match := range pathParameter.FindAllStringSubmatch(path, -1) {
				if !names[match[1]] {
					t.Errorf("undocumented path parameter %s", pattern)
				}
			}
			if method == "get" && op["requestBody"] != nil {
				t.Errorf("GET body would break Swagger/browser testing: %s", pattern)
			}
			security, explicit := op["security"].([]any)
			if path == "/health" || path == "/ready" {
				if !explicit || len(security) != 0 {
					t.Errorf("public probe not public: %s", path)
				}
			} else if strings.HasPrefix(path, "/api/v1/device/") {
				if !explicit || len(security) != 1 || security[0].(map[string]any)["ScannerBearer"] == nil {
					t.Errorf("scanner credential missing: %s", path)
				}
			} else if explicit {
				t.Errorf("unexpected authentication override: %s", path)
			}
			count++
		}
	}
	if count == 0 || count != len(registered) {
		t.Fatal("route coverage incomplete", count, len(registered))
	}
	t.Logf("All %d registered operations have exactly one correct module tag", count)
}

func TestPortableSnapshotAndReferences(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), JSON()) {
		t.Fatal("stale docs/openapi.json; run go run ./cmd/openapi -output docs/openapi.json")
	}
	spec := parsed(t)
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	var walk func(any)
	walk = func(v any) {
		switch node := v.(type) {
		case map[string]any:
			if value, ok := node["$ref"].(string); ok {
				if !strings.HasPrefix(value, "#/components/schemas/") || schemas[strings.TrimPrefix(value, "#/components/schemas/")] == nil {
					t.Errorf("unresolved or external ref %s", value)
				}
			}
			if required, ok := node["required"].([]any); ok {
				props, _ := node["properties"].(map[string]any)
				for _, field := range required {
					if props[field.(string)] == nil {
						t.Errorf("required property missing: %s", field)
					}
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(spec)
	for _, name := range []string{"RegisterStudent", "SubmitComplaint", "SubmitLateAccess", "ComplaintNote"} {
		if schemas[name].(map[string]any)["additionalProperties"] != false {
			t.Errorf("request model accepts unknown fields: %s", name)
		}
	}
	complaint := schemas["SubmitComplaint"].(map[string]any)["properties"].(map[string]any)
	if complaint["title"] != nil || complaint["severity"] != nil {
		t.Fatal("unimplemented complaint fields advertised")
	}
}

func TestSwaggerHTTP(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	for _, path := range []string{"/docs/", "/docs/initializer.js", "/docs/roles.css", "/docs/swagger-ui.css", "/docs/swagger-ui-bundle.js", "/openapi.json", "/docs/LICENSE", "/docs/NOTICE", "/docs/swagger-ui-bundle.js.LICENSE.txt"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(path, w.Code, w.Header())
		}
		if path != "/openapi.json" && !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
			t.Fatal("missing same-origin policy", path)
		}
	}
	for _, path := range []string{"/docs/unknown", "/docs/THIRD-PARTY.md", "/api/v1/unimplemented"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal(path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/docs", nil))
	if w.Code != 307 || w.Header().Get("Location") != "/docs/" {
		t.Fatal(w.Code, w.Header())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/openapi.json", nil))
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("HEAD", "/openapi.json", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal(w.Code, w.Body)
	}
	init, err := assets.ReadFile("assets/initializer.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(init, []byte("persistAuthorization: false")) || !bytes.Contains(init, []byte("validatorUrl: null")) || !bytes.Contains(init, []byte("queryConfigEnabled: false")) {
		t.Fatal("unsafe Swagger configuration")
	}
	for file, want := range map[string]string{
		"swagger-ui-bundle.js": "3f6abf70480db7e07914df4c98a77385f22ea585eaf457a3e6415ba0eae2d167",
		"swagger-ui.css":       "c3cfe8248c4b0b89673600fb829924eafbac6de4bc4b2b06f5f5243483074d0a",
	} {
		raw, err := assets.ReadFile("assets/" + file)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(raw)) != want {
			t.Fatalf("vendored asset changed: %s (%v)", file, err)
		}
	}
}
