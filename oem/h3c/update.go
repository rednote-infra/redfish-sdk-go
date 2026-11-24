package h3c

import (
	"net/url"
	"strings"

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
	delete(body, wire.FieldTransferProtocol)
	protocol := request.TransferProtocol
	if request.Username != "" && (strings.EqualFold(protocol, "sftp") || strings.EqualFold(protocol, "cifs")) {
		uri, err := url.Parse(request.ImageURI)
		if err != nil {
			return nil, err
		}
		if uri.User == nil {
			if request.Password != "" {
				uri.User = url.UserPassword(request.Username, request.Password)
			} else {
				uri.User = url.User(request.Username)
			}
			body[wire.FieldImageURI] = uri.String()
		}
	}
	public := provider.UpdateOptions(request, "RebootMode", "Backup", wire.FieldBiosFlash, "ImageMd5URI", "Date", "UpgradeType")
	if value, ok := request.Options[wire.FieldPreserve]; ok {
		public[wire.FieldPreserve] = value
	} else if request.PreserveConfig != nil {
		if *request.PreserveConfig {
			public[wire.FieldPreserve] = "Retain"
		} else {
			public[wire.FieldPreserve] = "Restore"
		}
	}
	provider.AddPublicUpdateOptions(body, public)
	return body, nil
}
