package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

func (c *Client) sessionsURI(ctx context.Context, attempted ...string) (string, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return "", sdkerrors.ErrClosed
	}
	options := RequestOptions{}
	if len(attempted) == 2 {
		request, _ := http.NewRequest(http.MethodGet, c.base.String(), nil)
		request.SetBasicAuth(attempted[0], attempted[1])
		options.Headers = request.Header
	}
	read := func(uri string) (*sdkresponse.Response, error) {
		if len(attempted) == 2 {
			return c.request(ctx, http.MethodGet, uri, nil, "", false, options)
		}
		return c.Request(ctx, http.MethodGet, uri, nil)
	}
	response, err := read(c.ServiceRoot())
	if err != nil {
		return "", err
	}
	var root models.RootService
	if err = response.Decode(&root); err != nil {
		return "", err
	}
	if root.Links != nil && root.Links.Sessions != nil && models.Value(root.Links.Sessions.ODataID) != "" {
		return models.Value(root.Links.Sessions.ODataID), nil
	}
	if root.SessionService == nil {
		return "", errors.New("service root does not advertise session service")
	}
	var service models.SessionService
	serviceResponse, err := read(models.Value(root.SessionService.ODataID))
	if err != nil {
		return "", err
	}
	if err = serviceResponse.Decode(&service); err != nil {
		return "", err
	}
	return requireLink(service.Sessions, "Sessions")
}

func (c *Client) Login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.tokenSnapshot() != "" {
		return nil
	}
	return c.loginLocked(ctx)
}

func (c *Client) refresh(ctx context.Context, expired string) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if token := c.tokenSnapshot(); token != "" && token != expired {
		return nil
	}
	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	username, password := c.credentials()
	_, err := c.createSession(ctx, username, password, true)
	return err
}

// CreateSession optionally installs the returned authentication token on this client.
func (c *Client) CreateSession(ctx context.Context, username, password string, switchToTokenAuth bool) (*models.Session, error) {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	return c.createSession(ctx, username, password, switchToTokenAuth)
}

func (c *Client) createSession(ctx context.Context, username, password string, install bool) (*models.Session, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, sdkerrors.ErrClosed
	}
	uri, err := c.sessionsURI(ctx, username, password)
	if err != nil {
		return nil, err
	}
	r, err := c.request(ctx, http.MethodPost, uri, Object{wire.FieldUserName: username, wire.FieldPassword: password}, "", true)
	if err != nil {
		if r != nil {
			return nil, sdkerrors.NewHTTPError(r, password, r.Header.Get(wire.HeaderAuthToken))
		}
		return nil, err
	}
	var session models.Session
	if err = r.Decode(&session); err != nil {
		return nil, err
	}
	token, location := r.Header.Get(wire.HeaderAuthToken), r.Header.Get(wire.HeaderLocation)
	if location == "" {
		location = models.Value(session.ODataID)
	}
	if location != "" {
		if _, err = c.resolve(location, r.URL); err != nil {
			return nil, err
		}
	}
	if token == "" {
		return nil, errors.New("session response is missing X-Auth-Token")
	}
	session.XAuthToken = models.Ptr(token)
	if session.ODataID == nil && location != "" {
		session.ODataID = &location
	}
	if session.ID == nil && location != "" {
		parsed, _ := url.Parse(location)
		id := models.StringOrNumber(path.Base(parsed.Path))
		session.ID = &id
	}
	rf := sdkerrors.NewHTTPError(r, password, token)
	restricted := errors.Is(rf, sdkerrors.ErrPasswordChangeRequired)
	if install || restricted {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return nil, sdkerrors.ErrClosed
		}
		c.mode = AuthenticationSession
		c.username = username
		c.password = password
		c.token = token
		c.sessionURI = location
	}
	if restricted {
		return &session, rf
	}
	return &session, nil
}

func (c *Client) GetSessions(ctx context.Context) ([]models.Session, error) {
	uri, err := c.sessionsURI(ctx)
	if err != nil {
		return nil, err
	}
	return collection[models.Session](ctx, c, uri)
}

func (c *Client) GetSession(ctx context.Context, id string) (*models.Session, error) {
	uri, err := c.sessionsURI(ctx)
	if err != nil {
		return nil, err
	}
	return selectResource[models.Session](ctx, c, uri, id)
}

func (c *Client) DeleteSession(ctx context.Context, id string) error {
	session, err := c.GetSession(ctx, id)
	if err != nil {
		return err
	}
	return c.Delete(ctx, models.Value(session.ODataID))
}
