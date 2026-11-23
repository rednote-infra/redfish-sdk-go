package discovery

import (
	"strings"
	"testing"
)

func TestSSDPUsesRedfishALAndRejectsUnrelatedOrDuplicateHeaders(t *testing.T) {
	valid := "HTTP/1.1 200 OK\r\nST: " + _ssdpServiceTarget + "\r\nUSN: uuid:12345678-1234-1234-1234-123456789abc::" + _ssdpServiceTarget + "\r\nAL: <https://example.invalid/redfish/v1/>\r\nLOCATION: http://other.invalid/upnp.xml\r\n\r\n"
	service, err := ParseSSDPResponse([]byte(valid), "192.0.2.1")
	if err != nil || service.Origin() != "https://example.invalid" {
		t.Fatalf("discovery: %+v %v", service, err)
	}
	if _, err = ParseSSDPResponse([]byte(strings.Replace(valid, "AL:", "ST: duplicate\r\nAL:", 1)), ""); err == nil {
		t.Fatal("duplicate ST accepted")
	}
	if _, err = ParseSSDPResponse([]byte(strings.Replace(valid, "AL:", "X-AL:", 1)), ""); err == nil {
		t.Fatal("LOCATION was used instead of AL")
	}
}
