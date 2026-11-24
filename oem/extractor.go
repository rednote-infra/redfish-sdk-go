package oem

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// NewExtractor selects a built-in vendor reading policy.
func NewExtractor(vendor Vendor) Extractor {
	// IEIT keeps the standard defaults used by the Client registry.
	if vendor == VendorIEIT {
		vendor = VendorGeneric
	}
	return extractor{vendor: vendor}
}

type extractor struct{ vendor Vendor }

func (p extractor) InletHistoryURI(thermal *models.Thermal) (string, error) {
	if p.vendor == VendorAMI {
		if models.Value(thermal.ODataID) == "" {
			return "", errors.New("Thermal is missing @odata.id")
		}
		return strings.TrimRight(models.Value(thermal.ODataID), "/") + "/Oem/Ami/InletHistoryTemperature", nil
	}
	if thermal.InletHistoryTemperature == nil || strings.TrimSpace(models.Value(thermal.InletHistoryTemperature.ODataID)) == "" {
		return "", fmt.Errorf("resource does not advertise InletHistoryTemperature: %w", sdkerrors.ErrNotFound)
	}
	return models.Value(thermal.InletHistoryTemperature.ODataID), nil
}

func (p extractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if fan.SpeedPercent != nil && fan.SpeedPercent.Reading != nil {
		return models.Ptr(*fan.SpeedPercent.Reading)
	}
	derive := func() *float64 {
		if fan.Reading != nil && fan.MaxReadingRange != nil && *fan.MaxReadingRange > 0 {
			return models.Ptr(math.Round(float64(*fan.Reading)/float64(*fan.MaxReadingRange)*1000) / 10)
		}
		return nil
	}
	switch p.vendor {
	case VendorXFusion:
		return oemNumber(fan.OEM, wire.OEMNamespaceXFusion, "SpeedRatio")
	case VendorZTE, VendorH3C:
		return oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
	case VendorInspur, VendorIEIT:
		primary := oemNumber(fan.OEM, wire.OEMNamespaceCustom, "SpeedRatio")
		if primary != nil && *primary != 0 {
			return primary
		}
		fallback := oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
		if fallback != nil {
			return fallback
		}
		return primary
	case VendorNettrix:
		ratio := oemNumber(fan.OEM, wire.FieldPublic, "SpeedRatio")
		if ratio != nil && *ratio != 0 {
			return ratio
		}
		return derive()
	default:
		return derive()
	}
}

func (p extractor) DriveTemperature(drive *models.Drive) *float64 {
	switch p.vendor {
	case VendorXFusion:
		return oemNumber(drive.OEM, wire.OEMNamespaceXFusion, "TemperatureCelsius")
	case VendorZTE, VendorH3C:
		return oemNumber(drive.OEM, wire.FieldPublic, "TemperatureCelsius")
	case VendorInspur, VendorIEIT, VendorLenovo:
		return oemNumber(drive.OEM, wire.FieldPublic, "temperature")
	case VendorNettrix:
		return drive.DiskTemperatureCelsius
	default:
		return nil
	}
}

func oemNumber(oem *models.Oem, namespace, key string) *float64 {
	if oem == nil {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal((*oem)[namespace], &fields) != nil {
		return nil
	}
	var number float64
	if json.Unmarshal(fields[key], &number) == nil {
		return &number
	}
	var text string
	if json.Unmarshal(fields[key], &text) == nil {
		if json.Unmarshal([]byte(text), &number) == nil {
			return &number
		}
	}
	return nil
}
