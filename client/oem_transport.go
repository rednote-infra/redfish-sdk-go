package client

import (
	"context"
	"net/http"

	"github.com/rednote-infra/redfish-sdk-go/client/provider"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// oemTransport exposes the existing Client authentication and I/O policies to OEM services.
type oemTransport struct{ *Client }

func (t oemTransport) Post(ctx context.Context, uri string, body any) (*sdkresponse.Response, error) {
	return t.Client.Post(ctx, uri, body)
}
func (t oemTransport) GetResponse(ctx context.Context, uri string) (*sdkresponse.Response, error) {
	return t.Client.Request(ctx, http.MethodGet, uri, nil)
}
func (t oemTransport) RedactKnownSecrets(message string) string {
	return redaction.String(message, t.passwordValue(), t.tokenSnapshot())
}

var _ provider.ResourceClient = oemTransport{}
var _ provider.DiagnosticResourceClient = oemTransport{}

func (t oemTransport) GetLogService(ctx context.Context, collectionURI, id string) (*models.Log, error) {
	services, err := collection[models.Log](ctx, t.Client, collectionURI)
	if err != nil {
		return nil, err
	}
	return selectItem(services, id)
}

// ResourceClient provides this Client's authenticated transport to OEM resource services.
func (c *Client) ResourceClient() provider.ResourceClient { return oemTransport{Client: c} }
