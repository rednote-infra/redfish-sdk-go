package response

import (
	"net/http"
	"testing"
)

func TestOperationRetainsResponseHeaders(t *testing.T) {
	response := &Response{StatusCode: http.StatusAccepted, Header: http.Header{"Link": []string{`</tasks/1>; rel="related"`}}, Body: []byte(`{"TaskState":"Running"}`)}
	operation, err := response.Operation()
	if err != nil {
		t.Fatal(err)
	}
	if operation.ResponseHeaders.Get("Link") != `</tasks/1>; rel="related"` {
		t.Fatal("lost task link")
	}
	response.Header.Set("Link", "changed")
	if operation.ResponseHeaders.Get("Link") == "changed" {
		t.Fatal("response headers were aliased")
	}
}
