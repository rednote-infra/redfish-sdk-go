package xfusion

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

// UpdateStrategy builds this vendor's firmware request.
type UpdateStrategy struct{}

func (UpdateStrategy) BuildBody(request provider.UpdateRequest) (provider.Object, error) {
	body, err := (provider.DefaultUpdateStrategy{}).BuildBody(request)
	if err != nil {
		return nil, err
	}
	delete(body, wire.FieldTargets)
	if request.PreserveConfig != nil {
		body["PreserveConfig"] = *request.PreserveConfig
	}
	for _, key := range []string{"ActiveMode", "ModuleArray"} {
		if value, ok := request.Options[key]; ok {
			body[key] = value
		}
	}
	return body, nil
}
