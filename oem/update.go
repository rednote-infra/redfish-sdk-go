package oem

import (
	"errors"
	"net/url"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// NewUpdateStrategy selects a built-in firmware payload builder.
func NewUpdateStrategy(vendor Vendor) UpdateStrategy {
	// IEIT keeps the standard defaults used by the Client registry.
	if vendor == VendorIEIT {
		vendor = VendorGeneric
	}
	return updateStrategy{vendor: vendor}
}

type UpdateRequest struct {
	ImageURI           string
	TransferProtocol   string
	Targets            []string
	Username, Password string
	PreserveConfig     *bool
	Options            Object
	Parameters         *models.UpdateParameters
	Vendor             Vendor
}

type updateStrategy struct{ vendor Vendor }

func (p updateStrategy) BuildBody(request UpdateRequest) (Object, error) {
	if request.ImageURI == "" {
		return nil, errors.New("image URI must not be empty")
	}
	protocol := request.TransferProtocol
	if protocol == "" {
		protocol = wire.TransferProtocolHTTP
	}
	body := Object{wire.FieldImageURI: request.ImageURI, wire.FieldTransferProtocol: protocol}
	public := Object{}
	copyOptions := func(keys ...string) {
		for _, key := range keys {
			if value, ok := request.Options[key]; ok {
				public[key] = value
			}
		}
	}
	credentials := func() {
		if request.Username != "" {
			body[wire.FieldUsername] = request.Username
		}
		if request.Password != "" {
			body[wire.FieldPassword] = request.Password
		}
	}
	targets := func() {
		if len(request.Targets) > 0 {
			body[wire.FieldTargets] = request.Targets
		}
	}
	preserve := func() {
		if request.PreserveConfig != nil {
			public[wire.FieldPreserveConf] = *request.PreserveConfig
		}
	}
	switch p.vendor {
	case VendorInspur, VendorIEIT:
		credentials()
		preserve()
		copyOptions(wire.FieldFlashItem, wire.FieldBIOSUpdateType, wire.FieldBiosFlash, "PFRType", "PFRRegion", "PFRUpdateDynamic", "SeamlessModule")
	case VendorZTE:
		preserve()
		copyOptions(wire.FieldBMCFlash, wire.FieldBiosFlash, wire.FieldApplyTime)
	case VendorEnginetech:
		if len(request.Targets) == 0 {
			return nil, errors.New("Enginetech firmware updates require explicit targets")
		}
		targets()
		credentials()
		preserve()
		copyOptions("ImageType")
	case VendorH3C:
		delete(body, wire.FieldTransferProtocol)
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
		if value, ok := request.Options[wire.FieldPreserve]; ok {
			public[wire.FieldPreserve] = value
		} else if request.PreserveConfig != nil {
			if *request.PreserveConfig {
				public[wire.FieldPreserve] = "Retain"
			} else {
				public[wire.FieldPreserve] = "Restore"
			}
		}
		copyOptions("RebootMode", "Backup", wire.FieldBiosFlash, "ImageMd5URI", "Date", "UpgradeType")
	case VendorNettrix:
		targets()
		if request.PreserveConfig != nil {
			body["SaveConfig"] = *request.PreserveConfig
		}
	case VendorXFusion:
		if request.PreserveConfig != nil {
			body["PreserveConfig"] = *request.PreserveConfig
		}
		for _, key := range []string{"ActiveMode", "ModuleArray"} {
			if value, ok := request.Options[key]; ok {
				body[key] = value
			}
		}
	case VendorLenovo:
		targets()
		credentials()
	default:
		targets()
	}
	if len(public) > 0 {
		body[wire.FieldOEM] = Object{wire.FieldPublic: public}
	}
	return body, nil
}
