package hpe

import (
	"errors"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
)

const _tpmOverrideFlag = "TPMOverrideFlag"

// UpdateStrategy builds an iLO SimpleUpdate request, including the explicit TPM override option.
type UpdateStrategy struct{}

func (UpdateStrategy) BuildBody(request provider.UpdateRequest) (provider.Object, error) {
	body, err := (provider.DefaultUpdateStrategy{}).BuildBody(request)
	if err != nil {
		return nil, err
	}
	if request.TransferProtocol == "" {
		delete(body, wire.FieldTransferProtocol)
	}
	provider.AddUpdateCredentials(body, request)
	if value, present := request.Options[_tpmOverrideFlag]; present {
		flag, ok := value.(bool)
		if !ok {
			return nil, errors.New("TPMOverrideFlag must be a boolean")
		}
		body[_tpmOverrideFlag] = flag
	}
	return body, nil
}
