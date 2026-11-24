package ami

import (
	"errors"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/models"
)

// Extractor overrides this vendor's resource links and readings.
type Extractor struct{ provider.DefaultExtractor }

func (Extractor) InletHistoryURI(thermal *models.Thermal) (string, error) {
	if models.Value(thermal.ODataID) == "" {
		return "", errors.New("Thermal is missing @odata.id")
	}
	return strings.TrimRight(models.Value(thermal.ODataID), "/") + "/Oem/Ami/InletHistoryTemperature", nil
}
