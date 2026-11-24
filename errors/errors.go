package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/internal/redaction"
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

var (
	ErrClosed                 = errors.New("client is closed")
	ErrNotFound               = errors.New("resource not found")
	ErrUnauthorized           = errors.New("authentication failed")
	ErrForbidden              = errors.New("access denied")
	ErrConflict               = errors.New("resource conflict")
	ErrPreconditionFailed     = errors.New("precondition failed")
	ErrRateLimit              = errors.New("rate limit exceeded")
	ErrPasswordChangeRequired = errors.New("password change required")
)

// Error retains the raw server response while exposing its usable message unchanged.
type Error struct {
	StatusCode         int
	Code, Message, URI string
	Body               []byte
	Header             http.Header
	ExtendedInfo       []models.RedfishErrorInfo
	cause              error
	display            string
}

func (e *Error) Error() string {
	if e.display != "" {
		return e.display
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("Redfish request failed with HTTP %d", e.StatusCode)
}

func (e *Error) Unwrap() error { return e.cause }

func IsStatus(err error, status int) bool {
	var rf *Error
	return errors.As(err, &rf) && rf.StatusCode == status
}

// NewHTTPError retains the raw response and redacts supplied secrets only for display.
func NewHTTPError(response *sdkresponse.Response, secrets ...string) *Error {
	var envelope struct {
		Error *struct {
			Code    string                    `json:"code"`
			Message string                    `json:"message"`
			Info    []models.RedfishErrorInfo `json:"@Message.ExtendedInfo"`
		} `json:"error"`
		Info []models.RedfishErrorInfo `json:"@Message.ExtendedInfo"`
	}
	// A malformed optional error property must not mask the original HTTP status.
	_ = json.Unmarshal(response.Body, &envelope)
	result := &Error{StatusCode: response.StatusCode, URI: response.URL, Body: append([]byte(nil), response.Body...), Header: response.Header.Clone(), ExtendedInfo: envelope.Info}
	if envelope.Error != nil {
		result.Code = envelope.Error.Code
		result.Message = envelope.Error.Message
		result.ExtendedInfo = append(result.ExtendedInfo, envelope.Error.Info...)
	}
	if strings.TrimSpace(result.Message) == "" {
		for _, info := range result.ExtendedInfo {
			if strings.TrimSpace(models.Value(info.Message)) != "" {
				result.Message = models.Value(info.Message)
				break
			}
		}
	}
	switch response.StatusCode {
	case http.StatusUnauthorized:
		result.cause = ErrUnauthorized
	case http.StatusForbidden:
		result.cause = ErrForbidden
	case http.StatusNotFound:
		result.cause = ErrNotFound
	case http.StatusConflict:
		result.cause = ErrConflict
	case http.StatusPreconditionFailed:
		result.cause = ErrPreconditionFailed
	case http.StatusTooManyRequests:
		result.cause = ErrRateLimit
	}
	for _, info := range result.ExtendedInfo {
		if strings.Contains(models.Value(info.MessageID), "PasswordChangeRequired") {
			result.cause = ErrPasswordChangeRequired
		}
	}
	if strings.HasSuffix(result.Code, "PasswordChangeRequired") {
		result.cause = ErrPasswordChangeRequired
	}
	// Credential redaction is confined to display strings; raw response attributes remain intact.
	if strings.TrimSpace(result.Message) != "" {
		result.display = redaction.String(result.Message, secrets...)
	}
	return result
}

type TransportError struct {
	Method, URI string
	Cause       error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("Redfish transport failed: %s %s", e.Method, e.URI)
}

func (e *TransportError) Unwrap() error { return e.Cause }

// Format keeps standard fmt logging from expanding raw response attributes.
func (e *Error) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		fmt.Fprintf(state, "%q", e.Error())
	} else {
		fmt.Fprint(state, e.Error())
	}
}

// MissingAttributeError reports an unusable typed response separately from HTTP failures.
type MissingAttributeError struct{ ResourceType, Attribute, URI string }

func (e *MissingAttributeError) Error() string {
	return fmt.Sprintf("%s response is missing %s", e.ResourceType, e.Attribute)
}
