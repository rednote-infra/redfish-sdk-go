package response

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rednote-infra/redfish-sdk-go/internal/wire"
	"github.com/rednote-infra/redfish-sdk-go/models"
	"github.com/rednote-infra/redfish-sdk-go/pointer"
)

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	URL        string
}

func (r *Response) Decode(dest any) error {
	if len(bytes.TrimSpace(r.Body)) == 0 {
		return nil
	}
	return json.Unmarshal(r.Body, dest)
}

func (r *Response) ETag() string { return r.Header.Get(wire.HeaderETag) }

func (r *Response) Operation() (*models.RedfishResponse, error) {
	result := &models.RedfishResponse{}
	if err := r.Decode(result); err != nil {
		return nil, err
	}
	result.HTTPStatus = pointer.Ptr(int64(r.StatusCode))
	if location := r.Header.Get(wire.HeaderLocation); location != "" {
		result.TaskMonitor = pointer.Ptr(location)
	}
	if retry := r.Header.Get(wire.HeaderRetryAfter); retry != "" {
		result.RetryAfter = pointer.Ptr(retry)
	}
	if result.TaskURI == nil {
		result.TaskURI = result.ODataID
	}
	return result, nil
}

func (r *Response) String() string {
	if r == nil {
		return "RedfishResponse(nil)"
	}
	return fmt.Sprintf("RedfishResponse(HTTP %d)", r.StatusCode)
}

func (r *Response) Format(state fmt.State, verb rune) { fmt.Fprint(state, r.String()) }
