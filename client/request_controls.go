package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

const _maxRedirects = 10

type RetrySafety string

const (
	RetrySafetyDefault    RetrySafety = "default"
	RetrySafetyIdempotent RetrySafety = "idempotent"
)

func validateRetry(policy RetryPolicy) error {
	if policy.MaxAttempts < 1 || policy.Backoff < 0 {
		return errors.New("invalid retry policy")
	}
	if policy.Safety != "" && policy.Safety != RetrySafetyDefault && policy.Safety != RetrySafetyIdempotent {
		return errors.New("unsupported retry safety policy")
	}
	return nil
}

func (c *Client) canReplay(method string, options ...RequestOptions) bool {
	policy := c.cfg.retry
	if len(options) == 1 && options[0].Retry != nil {
		policy = *options[0].Retry
	}
	return safeMethod(method) || policy.Safety == RetrySafetyIdempotent
}

type readDeadlineConn struct {
	net.Conn
	timeout time.Duration
}

func (c *readDeadlineConn) Read(buffer []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(buffer)
}

// requestClient applies connection/read overrides without mutating caller-owned transports.
func (c *Client) requestClient(options RequestOptions) (*http.Client, func(), error) {
	if options.ConnectTimeout < 0 || options.ReadTimeout < 0 {
		return nil, nil, errors.New("request timeouts must not be negative")
	}
	result := *c.http
	cleanup := func() {}
	if options.ConnectTimeout > 0 || options.ReadTimeout > 0 {
		original := result.Transport
		if original == nil {
			original = http.DefaultTransport
		}
		transport, ok := original.(*http.Transport)
		if !ok {
			return nil, nil, errors.New("per-request connection/read timeouts require an http.Transport")
		}
		clone := transport.Clone()
		wrapDial := func(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
			if dial == nil {
				dial = (&net.Dialer{}).DialContext
			}
			return func(ctx context.Context, network, address string) (net.Conn, error) {
				if options.ConnectTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, options.ConnectTimeout)
					defer cancel()
				}
				connection, err := dial(ctx, network, address)
				if err != nil {
					return nil, err
				}
				if options.ReadTimeout > 0 {
					return &readDeadlineConn{Conn: connection, timeout: options.ReadTimeout}, nil
				}
				return connection, nil
			}
		}
		clone.DialContext = wrapDial(clone.DialContext)
		if clone.DialTLSContext != nil {
			clone.DialTLSContext = wrapDial(clone.DialTLSContext)
		}
		if options.ReadTimeout > 0 {
			clone.ResponseHeaderTimeout = options.ReadTimeout
			result.Timeout = 0
		}
		result.Transport = clone
		cleanup = clone.CloseIdleConnections
	}
	if options.AllowRedirects {
		result.CheckRedirect = func(request *http.Request, previous []*http.Request) error {
			if len(previous) >= _maxRedirects {
				return errors.New("too many Redfish redirects")
			}
			_, err := c.resolve(request.URL.String(), c.base.String()+"/")
			return err
		}
	}
	return &result, cleanup, nil
}

func isReadTimeout(err error) bool {
	var network net.Error
	if !errors.As(err, &network) || !network.Timeout() {
		return false
	}
	var operation *net.OpError
	return !errors.As(err, &operation) || operation.Op != "dial"
}
