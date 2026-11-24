package nettrix

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
)

// UpdateStrategy builds this vendor's firmware request.
type UpdateStrategy struct{}

func (UpdateStrategy) BuildBody(request provider.UpdateRequest) (provider.Object, error) {
	body, err := (provider.DefaultUpdateStrategy{}).BuildBody(request)
	if err != nil {
		return nil, err
	}
	if request.PreserveConfig != nil {
		body["SaveConfig"] = *request.PreserveConfig
	}
	return body, nil
}
