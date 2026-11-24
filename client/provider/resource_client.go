package provider

import (
	"context"

	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// ResourceClient provides authenticated resource I/O to vendor services.
// RedactKnownSecrets applies the transport's existing credential-display boundary.
type ResourceClient interface {
	Get(context.Context, string, any) error
	GetManager(context.Context, ...string) (*models.Manager, error)
	Post(context.Context, string, any) (*sdkresponse.Response, error)
	GetResponse(context.Context, string) (*sdkresponse.Response, error)
	RedactKnownSecrets(string) string
}
