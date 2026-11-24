package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// CachedResource is a typed snapshot with the validator returned by the service.
// It is owned by the caller; concurrent updates require caller synchronization.
type CachedResource[T any] struct {
	Value       *T
	ETag        string
	NotModified bool
	Response    *sdkresponse.Response
	uri         string
}

// Fetch revalidates a prior snapshot of the same resource using If-None-Match.
// A 304 preserves the previous value; it never decodes an empty response body.
// Existing Get and Request error semantics are unchanged.
func Fetch[T any](ctx context.Context, c *Client, uri string, previous *CachedResource[T]) (*CachedResource[T], error) {
	headers := http.Header{}
	if previous != nil {
		if previous.uri != uri || previous.Value == nil {
			return nil, errors.New("cached resource does not match request")
		}
		if previous.ETag != "" {
			headers.Set(wire.HeaderIfNoneMatch, previous.ETag)
		}
	}
	response, err := c.Request(ctx, http.MethodGet, uri, nil, RequestOptions{Headers: headers, AcceptNotModified: true})
	if err != nil {
		return nil, err
	}
	result := &CachedResource[T]{ETag: response.ETag(), Response: response, uri: uri}
	if response.StatusCode == http.StatusNotModified {
		if previous == nil || previous.ETag == "" {
			return nil, errors.New("received 304 without a validated cached resource")
		}
		result.Value = previous.Value
		result.NotModified = true
		if result.ETag == "" {
			result.ETag = previous.ETag
		}
		return result, nil
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.New("conditional read returned an unexpected redirect")
	}
	var value T
	if err := json.Unmarshal(response.Body, &value); err != nil {
		return nil, err
	}
	if err := validateResourceResponse(&value, uri); err != nil {
		return nil, err
	}
	result.Value = &value
	return result, nil
}
