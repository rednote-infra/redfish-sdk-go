package lenovo

import (
	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// Extractor overrides this vendor's resource links and readings.
type Extractor struct{ provider.DefaultExtractor }

func (Extractor) DriveTemperature(drive *models.Drive) *float64 {
	return provider.OEMNumber(drive.OEM, wire.FieldPublic, wire.FieldLegacyTemperature)
}
