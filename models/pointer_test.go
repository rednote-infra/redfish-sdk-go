package models_test

import (
	"testing"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

func TestOptionalFieldHelpers(t *testing.T) {
	if models.Value(models.Ptr(false)) || models.Value(models.Ptr(0)) != 0 || models.Value(models.Ptr("")) != "" {
		t.Fatal("explicit zero values must be preserved")
	}
	if models.Value((*string)(nil)) != "" {
		t.Fatal("nil must return the zero value")
	}
	field := models.Ptr("initial")
	*field = "updated"
	if models.Value(field) != "updated" {
		t.Fatal("pointer must retain its value")
	}
}
