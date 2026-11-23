package redfish

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rednote-infra/redfish-sdk-go/models"
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
func newHTTPError(response *Response, secrets ...string) *Error {
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
			if strings.TrimSpace(Value(info.Message)) != "" {
				result.Message = Value(info.Message)
				break
			}
		}
	}
	switch response.StatusCode {
	case 401:
		result.cause = ErrUnauthorized
	case 403:
		result.cause = ErrForbidden
	case 404:
		result.cause = ErrNotFound
	case 409:
		result.cause = ErrConflict
	case 412:
		result.cause = ErrPreconditionFailed
	case 429:
		result.cause = ErrRateLimit
	}
	for _, info := range result.ExtendedInfo {
		if strings.Contains(Value(info.MessageID), "PasswordChangeRequired") {
			result.cause = ErrPasswordChangeRequired
		}
	}
	if strings.HasSuffix(result.Code, "PasswordChangeRequired") {
		result.cause = ErrPasswordChangeRequired
	}
	// Credential redaction is confined to display strings; raw response attributes remain intact.
	if strings.TrimSpace(result.Message) != "" {
		result.display = redactString(result.Message, secrets...)
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
func redactString(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

// Redact produces log-safe JSON without mutating the caller's raw data.
func Redact(value any, secrets ...string) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	var visit func(any) any
	visit = func(node any) any {
		switch v := node.(type) {
		case string:
			return redactString(v, secrets...)
		case map[string]any:
			for key, child := range v {
				lower := strings.ToLower(key)
				sensitive := false
				for _, word := range []string{"password", "secret", "token", "authorization", "authenticationkey", "encryptionkey", "cookie", "privatekey", "keytab"} {
					if strings.Contains(lower, word) {
						sensitive = true
						break
					}
				}
				if sensitive {
					v[key] = "[REDACTED]"
				} else {
					v[key] = visit(child)
				}
			}
		case []any:
			for i, child := range v {
				v[i] = visit(child)
			}
		}
		return node
	}
	return json.Marshal(visit(decoded))
}

// Format keeps standard fmt logging from expanding raw response attributes.
func (e *Error) Format(state fmt.State, verb rune) {
	if verb == 'q' {
		fmt.Fprintf(state, "%q", e.Error())
	} else {
		fmt.Fprint(state, e.Error())
	}
}

func requestSecrets(data []byte) []string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	var secrets []string
	var visit func(any)
	visit = func(node any) {
		switch fields := node.(type) {
		case map[string]any:
			for key, child := range fields {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "privatekey") || strings.Contains(lower, "keytab") {
					if text, ok := child.(string); ok && text != "" {
						secrets = append(secrets, text)
					}
				}
				visit(child)
			}
		case []any:
			for _, child := range fields {
				visit(child)
			}
		}
	}
	visit(value)
	return secrets
}

// MissingAttributeError reports an unusable typed response separately from HTTP failures.
type MissingAttributeError struct{ ResourceType, Attribute, URI string }

func (e *MissingAttributeError) Error() string {
	return fmt.Sprintf("%s response is missing %s", e.ResourceType, e.Attribute)
}
func validateResourceResponse(resource any, uri string) error {
	var kind string
	var identity bool
	switch value := resource.(type) {
	case *models.System:
		kind = "System"
		identity = strings.TrimSpace(Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "")
	case *models.Chassis:
		kind = "Chassis"
		identity = strings.TrimSpace(Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "")
	case *models.Task:
		kind = "Task"
		identity = strings.TrimSpace(Value(value.ODataID)) != "" || (value.ID != nil && strings.TrimSpace(string(*value.ID)) != "") || strings.TrimSpace(Value(value.TaskMonitor)) != ""
		if strings.TrimSpace(Value(value.TaskState)) == "" {
			return &MissingAttributeError{kind, FieldTaskState, uri}
		}
	default:
		return nil
	}
	if !identity {
		return &MissingAttributeError{kind, "@odata.id or Id", uri}
	}
	return nil
}
