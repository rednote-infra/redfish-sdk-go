package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

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

type vendorUpdate struct{ vendor Vendor }

func (p vendorUpdate) BuildBody(request UpdateRequest) (Object, error) {
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

func (c *Client) SimpleUpdate(ctx context.Context, request UpdateRequest) (*models.RedfishResponse, error) {
	vendor := request.Vendor
	if vendor == "" {
		var err error
		vendor, err = c.vendor(ctx)
		if err != nil {
			return nil, err
		}
	}
	body, err := c.cfg.providers.Update(vendor).BuildBody(request)
	if err != nil {
		return nil, err
	}
	service, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	action, err := findAction(service.Actions, wire.ActionSimpleUpdate)
	if err != nil {
		return nil, err
	}
	response, err := c.Post(ctx, action.Target, body)
	if err != nil {
		return nil, err
	}
	return response.Operation()
}

const (
	FlashFull                 = "FULLFwUpdate"
	FlashDualImage            = "DUALIMAGEFwUpdate"
	MultipartUpdateFile       = "UpdateFile"
	MultipartUpdateParameters = "UpdateParameters"
	MultipartOEMParameters    = "OemParameters"
)

type UploadOptions struct {
	FieldName     string
	Parameters    *models.UpdateParameters
	RawParameters Object
	OEMParameters Object
}

func (c *Client) updateAction(ctx context.Context, name string) (*Action, error) {
	service, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	return findAction(service.Actions, name)
}

func firmwareResult(r *sdkresponse.Response) (*models.FirmwareUpdateResponse, error) {
	var result models.FirmwareUpdateResponse
	if err := r.Decode(&result); err != nil {
		return nil, err
	}
	operation, err := r.Operation()
	if err != nil {
		return nil, err
	}
	result.HTTPStatus = operation.HTTPStatus
	result.TaskMonitor = operation.TaskMonitor
	result.TaskURI = operation.TaskURI
	result.RetryAfter = operation.RetryAfter
	return &result, nil
}

func (c *Client) UploadSignKey(ctx context.Context, path string) (*models.FirmwareUpdateResponse, error) {
	action, err := c.updateAction(ctx, wire.ActionUploadSignKey)
	if err != nil {
		return nil, err
	}
	r, err := c.uploadMultipart(ctx, action.Target, path, "pem_file", nil)
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}

func (c *Client) UploadFirmwareImage(ctx context.Context, path string) (*models.FirmwareUpdateResponse, error) {
	action, err := c.updateAction(ctx, wire.ActionUploadFirmwareImage)
	if err != nil {
		return nil, err
	}
	r, err := c.uploadMultipart(ctx, action.Target, path, "image_file", nil)
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}

func (c *Client) UploadFirmwareFile(ctx context.Context, path string, options UploadOptions) (*models.FirmwareUpdateResponse, error) {
	service, err := c.GetUpdateService(ctx)
	if err != nil {
		return nil, err
	}
	uri := pointer.Value(service.MultiPartHTTPPushURI)
	if uri == "" {
		uri = pointer.Value(service.HTTPPushURI)
	}
	if uri == "" {
		return nil, errors.New("update service does not advertise a push URI")
	}
	fields := map[string]json.RawMessage{}
	var parameters any = Object{}
	if options.RawParameters != nil {
		parameters = options.RawParameters
	}
	if options.Parameters != nil {
		if err = c.validateUpdateParameters(ctx, *options.Parameters); err != nil {
			return nil, err
		}
		parameters = options.Parameters
	}
	if service.MultiPartHTTPPushURI != nil || options.Parameters != nil || options.RawParameters != nil {
		data, err := json.Marshal(parameters)
		if err != nil {
			return nil, err
		}
		fields[MultipartUpdateParameters] = data
	}
	if options.OEMParameters != nil {
		data, err := json.Marshal(options.OEMParameters)
		if err != nil {
			return nil, err
		}
		fields[MultipartOEMParameters] = data
	}
	field := options.FieldName
	if field == "" {
		field = MultipartUpdateFile
	}
	r, err := c.uploadMultipart(ctx, uri, path, field, fields)
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}

func (c *Client) BuildUpdateParameters(ctx context.Context, parameters models.UpdateParameters) (*models.UpdateParameters, error) {
	if err := c.validateUpdateParameters(ctx, parameters); err != nil {
		return nil, err
	}
	return &parameters, nil
}

func (c *Client) validateUpdateParameters(ctx context.Context, parameters models.UpdateParameters) error {
	if parameters.Targets == nil && parameters.ExcludeTargets == nil && parameters.ForceUpdate == nil && parameters.Stage == nil && parameters.LocalImage == nil {
		return nil
	}
	capabilities, err := c.GetUpdateServiceCapabilities(ctx)
	if err != nil {
		return err
	}
	flags := []struct {
		requested, supported bool
		name                 string
	}{
		{requested: parameters.Targets != nil, supported: pointer.Value(capabilities.TargetsParameterSupported) || (capabilities.TargetsParameterSupported == nil && (capabilities.AllowableTargets != nil || len(capabilities.AllowableTargetReferenceTypes) > 0)), name: wire.FieldTargets},
		{requested: parameters.ExcludeTargets != nil, supported: pointer.Value(capabilities.ExcludeTargetsParameterSupported), name: wire.FieldExcludeTargets}, {requested: parameters.ForceUpdate != nil, supported: pointer.Value(capabilities.ForceUpdateParameterSupported), name: wire.FieldForceUpdate},
		{requested: parameters.Stage != nil, supported: pointer.Value(capabilities.StageParameterSupported) || (capabilities.StageParameterSupported == nil && capabilities.AllowableStaging != nil), name: wire.FieldStage}, {requested: parameters.LocalImage != nil, supported: pointer.Value(capabilities.LocalImageParameterSupported), name: wire.FieldLocalImage}}
	for _, flag := range flags {
		if flag.requested && !flag.supported {
			return fmt.Errorf("update capabilities do not advertise %s support", flag.name)
		}
	}
	for _, target := range parameters.Targets {
		if pointer.Value(parameters.Stage) {
			if !slices.Contains(capabilities.AllowableStaging, target) {
				return errors.New("staging target is not advertised")
			}
		} else if !slices.Contains(capabilities.AllowableTargets, target) {
			resolved, err := c.resolve(target, c.base.String()+"/")
			if err != nil {
				return err
			}
			var entity models.Entity
			if err = c.Get(ctx, resolved.String(), &entity); err != nil {
				return err
			}
			typ := strings.TrimPrefix(pointer.Value(entity.ODataType), "#")
			prefix := strings.Split(typ, ".")[0]
			if !slices.Contains(capabilities.AllowableTargetReferenceTypes, prefix) {
				return errors.New("update target reference type is not advertised")
			}
		}
	}
	return nil
}

func (c *Client) StandardSimpleUpdate(ctx context.Context, request UpdateRequest) (*models.FirmwareUpdateResponse, error) {
	parameters := models.UpdateParameters{}
	if request.Parameters != nil {
		parameters = *request.Parameters
	}
	if err := c.validateUpdateParameters(ctx, parameters); err != nil {
		return nil, err
	}
	data, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	var body Object
	if err = json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.ImageURI) == "" {
		return nil, errors.New("image URI must not be empty")
	}
	body[wire.FieldImageURI] = request.ImageURI
	protocol := request.TransferProtocol
	if protocol == "" {
		protocol = wire.TransferProtocolHTTP
	}
	body[wire.FieldTransferProtocol] = protocol
	if request.Username != "" {
		body[wire.FieldUsername] = request.Username
	}
	if request.Password != "" {
		body[wire.FieldPassword] = request.Password
	}
	action, err := c.updateAction(ctx, wire.ActionSimpleUpdate)
	if err != nil {
		return nil, err
	}
	r, err := c.Post(ctx, action.Target, body)
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}

func (c *Client) StageUpdate(ctx context.Context, request UpdateRequest) (*models.FirmwareUpdateResponse, error) {
	parameters := models.UpdateParameters{}
	if request.Parameters != nil {
		parameters = *request.Parameters
	}
	parameters.Stage = pointer.Ptr(true)
	if parameters.Targets == nil {
		parameters.Targets = request.Targets
	}
	if len(parameters.Targets) == 0 {
		return nil, errors.New("stage update requires explicit targets")
	}
	request.Parameters = &parameters
	if request.TransferProtocol == "" {
		uri, err := url.Parse(request.ImageURI)
		if err != nil {
			return nil, err
		}
		request.TransferProtocol = strings.ToUpper(uri.Scheme)
	}
	return c.StandardSimpleUpdate(ctx, request)
}

func (c *Client) ActivateSoftware(ctx context.Context, targets []string) (*models.FirmwareUpdateResponse, error) {
	if len(targets) == 0 {
		return nil, errors.New("activate requires at least one target")
	}
	links := make([]models.Link, 0, len(targets))
	for _, target := range targets {
		if _, err := c.resolve(target, c.base.String()+"/"); err != nil {
			return nil, err
		}
		links = append(links, models.Link{ODataID: pointer.Ptr(target)})
	}
	action, err := c.updateAction(ctx, wire.ActionActivate)
	if err != nil {
		return nil, err
	}
	r, err := c.Post(ctx, action.Target, Object{wire.FieldTargets: links})
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}

func (c *Client) BMCFirmwareUpdate(ctx context.Context, flashType, selector string) (*models.FirmwareUpdateResponse, error) {
	action, err := c.updateAction(ctx, wire.ActionBMCFwUpdate)
	if err != nil {
		return nil, err
	}
	if flashType == "" {
		flashType = FlashFull
	}
	var allowed []string
	if raw := action.Parameters["FlashType@Redfish.AllowableValues"]; raw != nil {
		if err = json.Unmarshal(raw, &allowed); err != nil {
			return nil, err
		}
	}
	if allowed == nil && action.ActionInfo == "" {
		allowed = []string{FlashFull, FlashDualImage}
	}
	if err = c.validateActionParameter(ctx, action.ActionInfo, "FlashType", flashType, allowed); err != nil {
		return nil, err
	}
	if flashType == FlashDualImage && selector == "" {
		return nil, errors.New("dual-image update requires UploadSelector")
	}
	body := Object{"FlashType": flashType}
	if selector != "" {
		var selectors []string
		if raw := action.Parameters["UploadSelector@Redfish.AllowableValues"]; raw != nil {
			if err = json.Unmarshal(raw, &selectors); err != nil {
				return nil, err
			}
		}
		if selectors == nil && action.ActionInfo == "" {
			selectors = []string{"AutoInactive", "Image1", "Image2", "ImageBoth"}
		}
		if err = c.validateActionParameter(ctx, action.ActionInfo, "UploadSelector", selector, selectors); err != nil {
			return nil, err
		}
		body["UploadSelector"] = selector
	}
	r, err := c.Post(ctx, action.Target, body)
	if err != nil {
		return nil, err
	}
	return firmwareResult(r)
}
