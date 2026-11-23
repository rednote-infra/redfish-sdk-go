package client

import (
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

func validateResourceResponse(resource any, uri string) error {
	var kind string
	var identity bool
	switch value := resource.(type) {
	case *models.System:
		kind = "System"
		identity = strings.TrimSpace(pointer.Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "")
	case *models.Chassis:
		kind = "Chassis"
		identity = strings.TrimSpace(pointer.Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "")
	case *models.Task:
		kind = "Task"
		identity = strings.TrimSpace(pointer.Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "") || strings.TrimSpace(pointer.Value(value.TaskMonitor)) != ""
		if strings.TrimSpace(pointer.Value(value.TaskState)) == "" {
			return &sdkerrors.MissingAttributeError{ResourceType: kind, Attribute: wire.FieldTaskState, URI: uri}
		}
	default:
		return nil
	}
	if !identity {
		return &sdkerrors.MissingAttributeError{ResourceType: kind, Attribute: "@odata.id or Id", URI: uri}
	}
	return nil
}
