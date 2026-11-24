package enginetech

import (
	"errors"

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
	if len(request.Targets) == 0 {
		return nil, errors.New("Enginetech firmware updates require explicit targets")
	}
	provider.AddUpdateCredentials(body, request)
	public := provider.UpdateOptions(request, "ImageType")
	if request.PreserveConfig != nil {
		public[wire.FieldPreserveConf] = *request.PreserveConfig
	}
	provider.AddPublicUpdateOptions(body, public)
	return body, nil
}
