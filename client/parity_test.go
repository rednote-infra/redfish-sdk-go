package client

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// This is a surface guard, not a claim that reflection verifies behavior.
func TestPythonPublicAPISurface(t *testing.T) {
	data, err := os.ReadFile("../testdata/api_parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Commit  string
		Methods []struct{ Python, Go string }
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Methods) != 188 || len(manifest.Commit) != 40 {
		t.Fatal("unexpected reference snapshot")
	}
	typ := reflect.TypeOf((*Client)(nil))
	seen := map[string]bool{}
	for _, method := range manifest.Methods {
		t.Run(method.Python, func(t *testing.T) {
			if seen[method.Python] {
				t.Fatal("duplicate source API")
			}
			seen[method.Python] = true
			if _, ok := typ.MethodByName(method.Go); !ok {
				t.Fatalf("Go SDK is missing %s", method.Go)
			}
		})
	}
}
