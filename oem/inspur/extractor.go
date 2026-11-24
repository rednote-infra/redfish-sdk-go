package inspur

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// Extractor overrides this vendor's resource links and readings.
type Extractor struct{ provider.DefaultExtractor }

func (Extractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if value := provider.ReportedFanSpeed(fan); value != nil {
		return value
	}
	primary := provider.OEMNumber(fan.OEM, wire.OEMNamespaceCustom, wire.FieldSpeedRatio)
	if primary != nil && *primary != 0 {
		return primary
	}
	if fallback := provider.OEMNumber(fan.OEM, wire.FieldPublic, wire.FieldSpeedRatio); fallback != nil {
		return fallback
	}
	return primary
}
func (Extractor) DriveTemperature(drive *models.Drive) *float64 {
	return provider.OEMNumber(drive.OEM, wire.FieldPublic, wire.FieldLegacyTemperature)
}
