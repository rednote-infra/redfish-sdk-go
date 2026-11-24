package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	sdkerrors "github.com/rednote-infra/redfish-sdk-go/errors"
	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

const _maxJSONBytes int64 = 64 << 20

type AuthenticationMode string

const (
	AuthenticationBasic   AuthenticationMode = "basic"
	AuthenticationSession AuthenticationMode = "session"
)

// Object is an explicitly constructed mutation or a genuinely dynamic resource.
// A nil map value sends JSON null; omitted keys are never synthesized.
type Object map[string]any

type RetryPolicy struct {
	// Safety permits mutation replay only when explicitly set to RetrySafetyIdempotent.
	Safety             RetrySafety
	RetryOnReadTimeout bool
	MaxAttempts        int
	Backoff            time.Duration
}

type config struct {
	client                *http.Client
	externalClient        *http.Client
	verifyTLS             bool
	caFile                string
	proxy                 string
	serviceRoot           string
	token                 string
	sessionURI            string
	timeout               time.Duration
	connectTimeout        time.Duration
	auth                  AuthenticationMode
	retry                 RetryPolicy
	vendor                Vendor
	providers             *ProviderRegistry
	strictCollections     bool
	collectionConcurrency int
}

type Option func(*config) error

func WithAuthentication(mode AuthenticationMode) Option {
	return func(c *config) error {
		if mode != AuthenticationBasic && mode != AuthenticationSession {
			return fmt.Errorf("unsupported authentication mode %q", mode)
		}
		c.auth = mode
		if mode == AuthenticationBasic {
			c.token = ""
			c.sessionURI = ""
		}
		return nil
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(c *config) error {
		if client == nil {
			return errors.New("HTTP client must not be nil")
		}
		c.client = client
		return nil
	}
}

func WithTLSVerification(verify bool) Option {
	return func(c *config) error { c.verifyTLS = verify; return nil }
}

func WithCABundle(path string) Option { return func(c *config) error { c.caFile = path; return nil } }

func WithProxy(proxy string) Option { return func(c *config) error { c.proxy = proxy; return nil } }

func WithTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		c.timeout = timeout
		return nil
	}
}

func WithConnectTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		if timeout <= 0 {
			return errors.New("connect timeout must be positive")
		}
		c.connectTimeout = timeout
		return nil
	}
}

func WithServiceRoot(path string) Option {
	return func(c *config) error { c.serviceRoot = path; return nil }
}

func WithRetry(policy RetryPolicy) Option {
	return func(c *config) error {
		if err := validateRetry(policy); err != nil {
			return err
		}
		c.retry = policy
		return nil
	}
}

func WithSession(token, location string) Option {
	return func(c *config) error {
		if token == "" {
			return errors.New("session token must not be empty")
		}
		c.auth = AuthenticationSession
		c.token = token
		c.sessionURI = location
		return nil
	}
}

func WithVendor(vendor Vendor) Option { return func(c *config) error { c.vendor = vendor; return nil } }

func WithProviders(registry *ProviderRegistry) Option {
	return func(c *config) error {
		if registry == nil {
			return errors.New("provider registry must not be nil")
		}
		c.providers = registry.Clone()
		return nil
	}
}

func WithStrictCollections(strict bool) Option {
	return func(c *config) error { c.strictCollections = strict; return nil }
}

// Client is safe for concurrent requests. Configuration is immutable after construction.
type Client struct {
	base               *url.URL
	http               *http.Client
	externalHTTP       *http.Client
	cfg                config
	username, password string
	mu                 sync.RWMutex
	token, sessionURI  string
	mode               AuthenticationMode
	closed             bool
	loginMu            sync.Mutex
	rootMu             sync.Mutex
	root               *models.RootService
	registryMu         sync.Mutex
	registryCache      map[string]*models.MessageRegistry
}

// NewClient accepts an origin URL or a hostname. TLS verification is enabled by default.
func NewClient(host, username, password string, options ...Option) (*Client, error) {
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	if (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("host must be an HTTP(S) origin without credentials, path, query, or fragment")
	}
	cfg := config{verifyTLS: true, timeout: 30 * time.Second, connectTimeout: 10 * time.Second, auth: AuthenticationBasic, serviceRoot: wire.DefaultServiceRoot, retry: RetryPolicy{MaxAttempts: 1, Backoff: time.Second}, providers: NewProviderRegistry()}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("option must not be nil")
		}
		if err = option(&cfg); err != nil {
			return nil, err
		}
	}
	var hc http.Client
	if cfg.client != nil {
		hc = *cfg.client
	} else {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DialContext = (&net.Dialer{Timeout: cfg.connectTimeout, KeepAlive: 30 * time.Second}).DialContext
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !cfg.verifyTLS} // Explicit compatibility option for BMC certificates.
		if cfg.caFile != "" {
			data, readErr := os.ReadFile(cfg.caFile)
			if readErr != nil {
				return nil, readErr
			}
			pool, poolErr := x509.SystemCertPool()
			if poolErr != nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(data) {
				return nil, errors.New("CA bundle contains no certificates")
			}
			transport.TLSClientConfig.RootCAs = pool
		}
		if cfg.proxy != "" {
			proxy, parseErr := url.Parse(cfg.proxy)
			if parseErr != nil || proxy.Host == "" {
				return nil, errors.New("invalid proxy URL")
			}
			transport.Proxy = http.ProxyURL(proxy)
		}
		hc.Transport = transport
		hc.Timeout = cfg.timeout
	}
	// Redirects are disabled by default; request-scoped following enforces same-origin URLs.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	externalHTTP, err := newExternalHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	c := &Client{base: base, http: &hc, externalHTTP: externalHTTP, cfg: cfg, username: username, password: password, mode: cfg.auth, token: cfg.token, sessionURI: cfg.sessionURI, registryCache: make(map[string]*models.MessageRegistry)}
	if _, err = c.resolve(cfg.serviceRoot, base.String()+"/"); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) Host() string { return c.base.Host }

func (c *Client) ServiceRoot() string { return c.cfg.serviceRoot }

func (c *Client) String() string { return "RedfishClient(" + c.base.Host + ")" }

func (c *Client) resolve(raw, parent string) (*url.URL, error) {
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if raw == "" || ref.User != nil || ref.Fragment != "" {
		return nil, errors.New("resource URL must not be empty or contain credentials or a fragment")
	}
	origin, err := url.Parse(parent)
	if err != nil {
		return nil, err
	}
	target := origin.ResolveReference(ref)
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	if !strings.EqualFold(target.Scheme, c.base.Scheme) || !strings.EqualFold(target.Hostname(), c.base.Hostname()) || port(target) != port(c.base) {
		return nil, errors.New("cross-origin resource URL is not allowed")
	}
	return target, nil
}

func (c *Client) tokenSnapshot() string { c.mu.RLock(); defer c.mu.RUnlock(); return c.token }

type RequestOptions struct {
	// Dial/read overrides require an http.Transport; custom RoundTrippers can use context deadlines.
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	// AllowRedirects follows same-origin URLs only.
	AllowRedirects bool
	// AcceptNotModified returns HTTP 304 with its headers for conditional reads.
	AcceptNotModified bool
	// ReturnRedirects exposes unfollowed 3xx responses without changing legacy defaults.
	ReturnRedirects bool
	Headers         http.Header
	Query           url.Values
	Retry           *RetryPolicy
}

func (c *Client) Request(ctx context.Context, method, uri string, body any, options ...RequestOptions) (*sdkresponse.Response, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, sdkerrors.ErrClosed
	}
	method = strings.ToUpper(method)
	if c.authenticationMode() == AuthenticationSession && c.tokenSnapshot() == "" {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
	}
	token := c.tokenSnapshot()
	result, err := c.request(ctx, method, uri, body, token, false, options...)
	// Only safe reads are replayed after authentication refresh; mutation outcomes may be uncertain.
	if sdkerrors.IsStatus(err, http.StatusUnauthorized) && c.authenticationMode() == AuthenticationSession && c.canReplay(method, options...) {
		if refreshErr := c.refresh(ctx, token); refreshErr != nil {
			return nil, refreshErr
		}
		return c.request(ctx, method, uri, body, c.tokenSnapshot(), false, options...)
	}
	return result, err
}

func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func (c *Client) request(ctx context.Context, method, uri string, body any, token string, anonymous bool, options ...RequestOptions) (*sdkresponse.Response, error) {
	if len(options) > 1 {
		return nil, errors.New("at most one request options value is allowed")
	}
	target, err := c.resolve(uri, c.base.String()+"/")
	if err != nil {
		return nil, err
	}
	var opt RequestOptions
	if len(options) == 1 {
		opt = options[0]
	}
	if opt.Query != nil {
		q := target.Query()
		for k, vs := range opt.Query {
			q.Del(k)
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		target.RawQuery = q.Encode()
	}
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	policy := c.cfg.retry
	if opt.Retry != nil {
		policy = *opt.Retry
	}
	if err := validateRetry(policy); err != nil {
		return nil, err
	}
	if !safeMethod(method) && policy.Safety != RetrySafetyIdempotent {
		policy.MaxAttempts = 1
	}
	httpClient, cleanup, err := c.requestClient(opt)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set(wire.HeaderODataVersion, wire.ODataVersion)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if !anonymous {
			if token != "" {
				req.Header.Set(wire.HeaderAuthToken, token)
			} else {
				username, password := c.credentials()
				req.SetBasicAuth(username, password)
			}
		}
		for k, vs := range opt.Headers {
			req.Header.Del(k)
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		raw, sendErr := httpClient.Do(req)
		if sendErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if policy.RetryOnReadTimeout && attempt+1 < policy.MaxAttempts && isReadTimeout(sendErr) {
				if err := waitContext(ctx, retryDelay(nil, policy.Backoff, attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, &sdkerrors.TransportError{Method: method, URI: target.EscapedPath(), Cause: sendErr}
		}
		content, readErr := io.ReadAll(io.LimitReader(raw.Body, _maxJSONBytes+1))
		closeErr := raw.Body.Close()
		result := &sdkresponse.Response{StatusCode: raw.StatusCode, Header: raw.Header.Clone(), Body: content, URL: target.String()}
		if (opt.AcceptNotModified && raw.StatusCode == http.StatusNotModified) || (opt.ReturnRedirects && raw.StatusCode >= http.StatusMultipleChoices && raw.StatusCode < http.StatusBadRequest) {
			return result, nil
		}
		if raw.StatusCode < 200 || raw.StatusCode >= 300 {
			secrets := append(redaction.Secrets(data), c.passwordValue(), token, req.Header.Get(wire.HeaderAuthToken), req.Header.Get("Authorization"))
			rf := sdkerrors.NewHTTPError(result, secrets...)
			if attempt+1 < policy.MaxAttempts && retryableStatus(raw.StatusCode) {
				delay := retryDelay(result.Header, policy.Backoff, attempt)
				if err = waitContext(ctx, delay); err != nil {
					return nil, err
				}
				continue
			}
			return result, rf
		}
		if readErr != nil {
			if policy.RetryOnReadTimeout && attempt+1 < policy.MaxAttempts && isReadTimeout(readErr) && ctx.Err() == nil {
				if err := waitContext(ctx, retryDelay(nil, policy.Backoff, attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return result, &sdkerrors.TransportError{Method: method, URI: target.EscapedPath(), Cause: readErr}
		}
		if closeErr != nil {
			return result, closeErr
		}
		if int64(len(content)) > _maxJSONBytes {
			return result, errors.New("JSON response exceeds size limit")
		}
		return result, nil
	}
	return nil, errors.New("request attempts exhausted")
}

func retryableStatus(status int) bool {
	return status == 429 || status == 502 || status == 503 || status == 504
}

func retryDelay(headers http.Header, backoff time.Duration, attempt int) time.Duration {
	if value := headers.Get(wire.HeaderRetryAfter); value != "" {
		var seconds int
		if _, err := fmt.Sscanf(value, "%d", &seconds); err == nil && seconds >= 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			return max(time.Duration(0), time.Until(when))
		}
	}
	for i := 0; i < attempt && backoff < 30*time.Second; i++ {
		backoff *= 2
	}
	return min(backoff, 30*time.Second)
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) Get(ctx context.Context, uri string, dest any) error {
	r, err := c.Request(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(r.Body)) == 0 {
		return errors.New("empty JSON resource response")
	}
	if err = r.Decode(dest); err != nil {
		return err
	}
	return validateResourceResponse(dest, uri)
}

func (c *Client) GetRaw(ctx context.Context, uri string) (Object, error) {
	var result Object
	err := c.Get(ctx, uri, &result)
	return result, err
}

func (c *Client) Patch(ctx context.Context, uri string, body any, options ...RequestOptions) (*sdkresponse.Response, error) {
	return c.Request(ctx, http.MethodPatch, uri, body, options...)
}

func (c *Client) Post(ctx context.Context, uri string, body any, options ...RequestOptions) (*sdkresponse.Response, error) {
	return c.Request(ctx, http.MethodPost, uri, body, options...)
}

func (c *Client) Delete(ctx context.Context, uri string, options ...RequestOptions) error {
	_, err := c.Request(ctx, http.MethodDelete, uri, nil, options...)
	return err
}

func (c *Client) PatchResponse(ctx context.Context, uri string, body any, options ...RequestOptions) (*sdkresponse.Response, error) {
	return c.Patch(ctx, uri, body, options...)
}

func (c *Client) PostResponse(ctx context.Context, uri string, body any, options ...RequestOptions) (*sdkresponse.Response, error) {
	return c.Post(ctx, uri, body, options...)
}

func (c *Client) DeleteResponse(ctx context.Context, uri string, options ...RequestOptions) (*sdkresponse.Response, error) {
	return c.Request(ctx, http.MethodDelete, uri, nil, options...)
}

func (c *Client) GetETag(ctx context.Context, uri string) (string, error) {
	r, err := c.Request(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return "", err
	}
	if tag := r.ETag(); tag != "" {
		return tag, nil
	}
	var entity models.Entity
	if err = r.Decode(&entity); err != nil {
		return "", err
	}
	return models.Value(entity.ODataETag), nil
}

func (c *Client) Close() error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	token, location := c.token, c.sessionURI
	c.token = ""
	c.sessionURI = ""
	c.mu.Unlock()
	defer c.http.CloseIdleConnections()
	if c.cfg.externalClient == nil {
		defer c.externalHTTP.CloseIdleConnections()
	}
	if token == "" || location == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := c.request(ctx, http.MethodDelete, location, nil, token, false)
	return err
}

func (c *Client) authenticationMode() AuthenticationMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

func (c *Client) credentials() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.username, c.password
}

func (c *Client) passwordValue() string { _, password := c.credentials(); return password }

func (c *Client) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		fmt.Fprintf(state, "%q", c.String())
	} else {
		fmt.Fprint(state, c.String())
	}
}

// WithCollectionConcurrency bounds concurrent member reads. The default is one.
// Members and diagnostics retain the service's advertised order.
func WithCollectionConcurrency(workers int) Option {
	return func(c *config) error {
		if workers < 1 {
			return errors.New("collection concurrency must be positive")
		}
		c.collectionConcurrency = workers
		return nil
	}
}
