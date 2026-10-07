package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Guard the boundaries that make feature modules independently maintainable.
func TestModuleDependencyBoundaries(t *testing.T) {
	const prefix = "github.com/Hostel-Hive/hostelhive-backend/internal/"
	root := filepath.Join("..", "..", "internal")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 4 || parts[0] != "modules" {
			return nil
		}
		module, layer := parts[1], parts[2]
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dependency, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			local := strings.TrimPrefix(dependency, prefix)
			concrete := strings.HasPrefix(local, "platform/") || strings.Contains(local, "/repository") || strings.HasPrefix(local, "app") || strings.HasPrefix(local, "workers")
			sdk := strings.HasPrefix(dependency, "firebase.google.com/") || strings.HasPrefix(dependency, "github.com/jackc/") || strings.HasPrefix(dependency, "github.com/aws/")
			if (layer == "handler" || layer == "service" || layer == "domain" || layer == "dto") && (concrete || sdk) {
				t.Errorf("%s imports concrete dependency %s", relative, dependency)
			}
			if (layer == "domain" || layer == "dto" || layer == "service" || layer == "repository") && dependency == "net/http" {
				t.Errorf("%s mixes HTTP transport into %s", relative, layer)
			}
			if strings.HasPrefix(local, "modules/") {
				other := strings.Split(local, "/")
				if len(other) > 3 && other[1] != module && other[2] == "repository" {
					t.Errorf("%s imports another module's repository", relative)
				}
			}
			if layer == "domain" && strings.HasPrefix(dependency, prefix) {
				t.Errorf("%s domain depends on %s", relative, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
