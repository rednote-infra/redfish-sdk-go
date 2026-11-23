package redfish_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryRootContainsOnlyCompatibilityDeclarations(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if method, ok := decl.(*ast.FuncDecl); ok && method.Recv != nil {
				t.Errorf("implementation method %s remains at root in %s", method.Name.Name, name)
			}
		}
	}
	for _, directory := range []string{"client", "errors", "response", "discovery"} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			t.Errorf("missing package directory %s", directory)
		}
	}
}
