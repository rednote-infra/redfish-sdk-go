package dell

import "github.com/rednote-infra/redfish-sdk-go/client/provider"

// UpdateStrategy retains standard Dell targets and optional image source credentials.
type UpdateStrategy struct{}

func (UpdateStrategy) BuildBody(request provider.UpdateRequest) (provider.Object, error) {
	body, err := (provider.DefaultUpdateStrategy{}).BuildBody(request)
	if err != nil {
		return nil, err
	}
	provider.AddUpdateCredentials(body, request)
	return body, nil
}
