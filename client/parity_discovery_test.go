package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
)

func TestDiscoveryStaysInsideSubtreeAndPaginationDoesNotConsumeDepth(t *testing.T) {
	var requests []string
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		switch r.URL.RequestURI() {
		case "/scope":
			fmt.Fprint(w, `{"@odata.id":"/scope","@odata.type":"#ComputerSystemCollection.ComputerSystemCollection","Members":[],"Members@odata.nextLink":"/scope?p=2","Peer":{"@odata.id":"/outside"}}`)
		case "/scope?p=2":
			fmt.Fprint(w, `{"@odata.id":"/scope","@odata.type":"#ComputerSystemCollection.ComputerSystemCollection","Members":[],"Members@odata.nextLink":"/scope?p=3"}`)
		case "/scope?p=3":
			fmt.Fprint(w, `{"@odata.id":"/scope","@odata.type":"#ComputerSystemCollection.ComputerSystemCollection","Members":[]}`)
		default:
			t.Errorf("out-of-scope request: %s", r.URL)
			http.NotFound(w, r)
		}
	})
	index, err := sdk.DiscoverResources(context.Background(), DiscoveryOptions{Start: "/scope", MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 || len(index.Resources) != 1 || index.Truncated || !slices.Contains(index.Excluded, "/outside") {
		t.Fatalf("discovery contract: requests=%v index=%+v", requests, index)
	}
	filtered := index.Find("ComputerSystemCollection", nil)
	if !slices.Equal(filtered.Excluded, index.Excluded) {
		t.Fatal("filter lost exclusion diagnostics")
	}
}

func TestDiscoveryReportsMissingIdentityAndBoundsRejectedLinks(t *testing.T) {
	sdk, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"@odata.id":"/scope","@odata.type":"#System.v1_0_0.System","Links":[{"@odata.id":"http://outside.invalid/a"},{"@odata.id":"http://outside.invalid/b"},{"@odata.id":"http://outside.invalid/c"}]}`)
	})
	index, err := sdk.DiscoverResources(context.Background(), DiscoveryOptions{Start: "/scope", MaxResources: 2})
	if err != nil || !index.Truncated || len(index.Failures) > 2 {
		t.Fatalf("unbounded failures: %+v %v", index, err)
	}
}
