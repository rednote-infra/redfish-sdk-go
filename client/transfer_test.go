package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func TestDownloadAtomicAndSafeFilename(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../../bundle.tar.gz"`)
		fmt.Fprint(w, "archive")
	})
	path := filepath.Join(t.TempDir(), "bundle.tar.gz")
	actual, err := c.Download(context.Background(), "/archive", path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(actual)
	if err != nil || string(data) != "archive" {
		t.Fatalf("bad archive: %q %v", data, err)
	}
}

func TestMultipartFirmwarePreservesFileAndJSONFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.bin")
	if err := os.WriteFile(file, []byte("firmware"), 0600); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			fmt.Fprint(w, `{"UpdateService":{"@odata.id":"/update"}}`)
		case "/update":
			fmt.Fprint(w, `{"MultipartHttpPushUri":"/upload"}`)
		case "/upload":
			if err := r.ParseMultipartForm(1024); err != nil {
				t.Error(err)
				return
			}
			image, _, err := r.FormFile("UpdateFile")
			if err != nil {
				t.Error(err)
				return
			}
			defer image.Close()
			data, _ := io.ReadAll(image)
			if string(data) != "firmware" {
				t.Error("wrong file content")
			}
			if r.FormValue("UpdateParameters") != "{}" {
				t.Errorf("parameters: %q", r.FormValue("UpdateParameters"))
			}
			w.Header().Set(wire.HeaderLocation, "/monitor")
			w.WriteHeader(202)
			fmt.Fprint(w, `{"@odata.id":"/tasks/1"}`)
		default:
			http.NotFound(w, r)
		}
	})
	result, err := c.UploadFirmwareFile(context.Background(), file, UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pointer.Value(result.TaskMonitor) != "/monitor" {
		t.Fatal("monitor metadata was lost")
	}
}
