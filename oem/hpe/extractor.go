package hpe

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// Extractor preserves HPE fan readings already expressed as percentages.
type Extractor struct{ provider.DefaultExtractor }

func (p Extractor) FanSpeedRatio(fan *models.Fan) *float64 {
	if value := provider.ReportedFanSpeed(fan); value != nil {
		return value
	}
	if models.Value(fan.ReadingUnits) == wire.ReadingUnitsPercent && fan.Reading != nil {
		return models.Ptr(float64(*fan.Reading))
	}
	return p.DefaultExtractor.FanSpeedRatio(fan)
}
