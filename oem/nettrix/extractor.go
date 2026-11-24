package nettrix

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// Extractor overrides this vendor's resource links and readings.
type Extractor struct{ provider.DefaultExtractor }

func (p Extractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if value := provider.ReportedFanSpeed(fan); value != nil {
		return value
	}
	if value := provider.OEMNumber(fan.OEM, wire.FieldPublic, wire.FieldSpeedRatio); value != nil && *value != 0 {
		return value
	}
	return p.DefaultExtractor.FanSpeedRatio(fan)
}
func (Extractor) DriveTemperature(drive *models.Drive) *float64 { return drive.DiskTemperatureCelsius }
