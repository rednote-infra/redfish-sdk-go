package zte

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
	public := provider.UpdateOptions(request, wire.FieldBMCFlash, wire.FieldBiosFlash, wire.FieldApplyTime)
	if request.PreserveConfig != nil {
		public[wire.FieldPreserveConf] = *request.PreserveConfig
	}
	provider.AddPublicUpdateOptions(body, public)
	return body, nil
}
