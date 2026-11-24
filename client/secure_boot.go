package client

import (
	"context"
	"errors"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func (c *Client) GetSecureBootDatabases(ctx context.Context, systemID ...string) ([]models.SecureBootDatabase, error) {
	secure, err := c.GetSecureBoot(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	return linkedCollection[models.SecureBootDatabase](ctx, c, secure.SecureBootDatabases, "SecureBootDatabases")
}
func (c *Client) GetSecureBootDatabase(ctx context.Context, id string, systemID ...string) (*models.SecureBootDatabase, error) {
	databases, err := c.GetSecureBootDatabases(ctx, systemID...)
	if err != nil {
		return nil, err
	}
	return selectItem(databases, id)
}

// AddSecureBootCertificate returns the complete enrollment response, including Location/task headers.
// It does not assume that a synchronous response contains a Certificate body.
func (c *Client) AddSecureBootCertificate(ctx context.Context, databaseID string, request models.SecureBootCertificateRequest, systemID ...string) (*sdkresponse.Response, error) {
	if strings.TrimSpace(request.CertificateString) == "" || request.CertificateType == "" {
		return nil, errors.New("certificate string and type are required")
	}
	database, err := c.GetSecureBootDatabase(ctx, databaseID, systemID...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(database.Certificates, "Certificates")
	if err != nil {
		return nil, err
	}
	return c.Post(ctx, uri, request)
}
func (c *Client) AddSecureBootSignature(ctx context.Context, databaseID string, request models.SecureBootSignatureRequest, systemID ...string) (*sdkresponse.Response, error) {
	if request.SignatureString == "" || request.SignatureTypeRegistry == "" || request.SignatureType == "" {
		return nil, errors.New("signature string, registry, and type are required")
	}
	database, err := c.GetSecureBootDatabase(ctx, databaseID, systemID...)
	if err != nil {
		return nil, err
	}
	uri, err := requireLink(database.Signatures, "Signatures")
	if err != nil {
		return nil, err
	}
	return c.Post(ctx, uri, request)
}
