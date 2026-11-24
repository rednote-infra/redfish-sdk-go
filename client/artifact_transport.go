package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// WithExternalHTTPClient supplies a distinct caller-owned transport for external artifacts.
func WithExternalHTTPClient(httpClient *http.Client) Option {
	return func(cfg *config) error {
		if httpClient == nil {
			return errors.New("external HTTP client must not be nil")
		}
		cfg.externalClient = httpClient
		return nil
	}
}

func validateArtifactURL(raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if (target.Scheme != "https" && target.Scheme != "http") || target.Host == "" || target.User != nil || target.Fragment != "" {
		return nil, errors.New("external artifact must be an HTTP(S) URL without credentials or a fragment")
	}
	return target, nil
}

func newExternalHTTPClient(cfg config) (*http.Client, error) {
	var result http.Client
	if cfg.externalClient != nil {
		if cfg.externalClient == cfg.client {
			return nil, errors.New("external and BMC HTTP clients must be distinct")
		}
		result = *cfg.externalClient
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !cfg.verifyTLS} // Explicit BMC compatibility also applies to artifact trust.
		if cfg.caFile != "" {
			data, err := os.ReadFile(cfg.caFile)
			if err != nil {
				return nil, err
			}
			pool, err := x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(data) {
				return nil, errors.New("CA bundle contains no certificates")
			}
			transport.TLSClientConfig.RootCAs = pool
		}
		if cfg.proxy != "" {
			proxy, err := url.Parse(cfg.proxy)
			if err != nil {
				return nil, err
			}
			transport.Proxy = http.ProxyURL(proxy)
		}
		result.Transport = transport
		result.Timeout = cfg.timeout
	}
	result.CheckRedirect = func(request *http.Request, previous []*http.Request) error {
		if len(previous) >= _maxRedirects {
			return errors.New("too many artifact redirects")
		}
		if _, err := validateArtifactURL(request.URL.String()); err != nil {
			return err
		}
		// External requests never inherit BMC authorization or cookies.
		request.Header.Del("Authorization")
		request.Header.Del(wire.HeaderAuthToken)
		request.Header.Del("Cookie")
		return nil
	}
	return &result, nil
}

func (c *Client) downloadResponse(ctx context.Context, method, uri string, body io.Reader, contentType string) (*http.Response, error) {
	for redirects := 0; ; redirects++ {
		target, resolveErr := c.resolve(uri, c.base.String()+"/")
		if resolveErr != nil {
			break
		}
		response, err := c.stream(ctx, method, target.String(), body, contentType)
		var redirect *sdkerrors.Error
		if method != http.MethodGet || !errors.As(err, &redirect) || redirect.StatusCode < http.StatusMultipleChoices || redirect.StatusCode >= http.StatusBadRequest || redirect.Header.Get(wire.HeaderLocation) == "" {
			return response, err
		}
		if redirects >= _maxRedirects {
			return nil, errors.New("too many artifact redirects")
		}
		next, err := url.Parse(redirect.Header.Get(wire.HeaderLocation))
		if err != nil {
			return nil, err
		}
		uri = target.ResolveReference(next).String()
		if _, err := validateArtifactURL(uri); err != nil {
			return nil, err
		}
	}
	target, err := validateArtifactURL(uri)
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, sdkerrors.ErrClosed
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := c.externalHTTP.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &sdkerrors.TransportError{Method: method, URI: target.EscapedPath(), Cause: err}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, _maxJSONBytes))
		return nil, sdkerrors.NewHTTPError(&sdkresponse.Response{StatusCode: response.StatusCode, Header: response.Header, Body: data, URL: target.String()}, c.passwordValue(), c.tokenSnapshot())
	}
	return response, nil
}

// DownloadBytes returns artifact contents without creating a local file.
func (c *Client) DownloadBytes(ctx context.Context, uri string) ([]byte, error) {
	response, err := c.downloadResponse(ctx, http.MethodGet, uri, nil, "")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}
