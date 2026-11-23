package main

import (
	"strings"
	"testing"
)

func TestValidateResults(t *testing.T) {
	tests := []struct {
		name      string
		events    string
		wantError bool
	}{
		{
			name: "constant-only package has no tests to skip",
			events: `{"Action":"pass","Test":"TestClient","Package":"client"}
{"Action":"pass","Package":"client"}
{"Action":"skip","Package":"wire"}`,
		},
		{
			name: "explicit skipped test is rejected",
			events: `{"Action":"pass","Test":"TestClient","Package":"client"}
{"Action":"skip","Test":"TestSkipped","Package":"client"}
{"Action":"pass","Package":"client"}`,
			wantError: true,
		},
		{
			name:      "failed test is rejected",
			events:    `{"Action":"fail","Test":"TestClient","Package":"client"}`,
			wantError: true,
		},
		{
			name:      "empty results are rejected",
			wantError: true,
		},
		{
			name:      "malformed results are rejected",
			events:    `invalid JSON`,
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateResults(strings.NewReader(tt.events))
			if (err != nil) != tt.wantError {
				t.Fatalf("validateResults() error = %v, want error = %v", err, tt.wantError)
			}
		})
	}
}
