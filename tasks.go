package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rednote-infra/redfish-sdk-go/models"
)

const (
	TaskNew         = "New"
	TaskRunning     = "Running"
	TaskPending     = "Pending"
	TaskCompleted   = "Completed"
	TaskException   = "Exception"
	TaskKilled      = "Killed"
	TaskCancelled   = "Cancelled"
	TaskInterrupted = "Interrupted"
)

type PollOptions struct {
	Interval, Timeout        time.Duration
	MaxTransientFailures     int
	AllowNotFoundRecovery    bool
	AllowTaskVisibilityDelay bool
	initialDelay             time.Duration
}
type AsyncOperationError struct {
	Task     *models.Task
	Response *Response
	Message  string
}

func (e *AsyncOperationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "asynchronous operation failed"
}

type OperationResult struct {
	Task     *models.Task
	Response *Response
}

func taskFailure(task *models.Task) error {
	state := Value(task.TaskState)
	switch state {
	case TaskException, TaskKilled, TaskCancelled, TaskInterrupted:
		return &AsyncOperationError{Task: task, Message: "Redfish task terminated with state " + state}
	}
	if state == TaskCompleted && Value(task.TaskStatus) == "Critical" {
		return &AsyncOperationError{Task: task, Message: "Redfish task completed with critical status"}
	}
	return nil
}
func pollContext(ctx context.Context, options PollOptions) (context.Context, context.CancelFunc, time.Duration, error) {
	if options.Interval < 0 || options.Timeout < 0 || options.MaxTransientFailures < 0 {
		return nil, nil, 0, errors.New("poll options must not be negative")
	}
	interval := options.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, interval, nil
}
func (c *Client) WaitForTask(ctx context.Context, id string, options PollOptions) (*models.Task, error) {
	if id == "" {
		return nil, errors.New("task identifier must not be empty")
	}
	uri := id
	if id[0] != '/' {
		task, err := c.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		uri = Value(task.ODataID)
	}
	result, err := c.pollOperation(ctx, uri, false, options)
	if result == nil {
		return nil, err
	}
	return result.Task, err
}
func (c *Client) WaitForOperation(ctx context.Context, operation *models.RedfishResponse, options PollOptions) (*OperationResult, error) {
	if operation == nil {
		return nil, errors.New("operation must not be nil")
	}
	uri := Value(operation.TaskMonitor)
	monitor := uri != ""
	if uri == "" {
		uri = Value(operation.TaskURI)
	}
	if uri == "" {
		uri = Value(operation.ODataID)
	}
	if uri == "" {
		if Value(operation.HTTPStatus) == http.StatusAccepted {
			return nil, errors.New("accepted operation does not advertise a task monitor or task URI")
		}
		return &OperationResult{}, nil
	}
	if retry := Value(operation.RetryAfter); retry != "" {
		headers := http.Header{}
		headers.Set(HeaderRetryAfter, retry)
		options.initialDelay = retryDelay(headers, 0, 0)
	}
	return c.pollOperation(ctx, uri, monitor, options)
}
func (c *Client) pollOperation(ctx context.Context, uri string, monitor bool, options PollOptions) (*OperationResult, error) {
	ctx, cancel, interval, err := pollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if options.initialDelay > 0 {
		if err = waitContext(ctx, options.initialDelay); err != nil {
			return nil, err
		}
	}
	failures := 0
	for {
		response, err := c.Request(ctx, http.MethodGet, uri, nil)
		if err != nil {
			retryable := IsStatus(err, 429) || IsStatus(err, 503) || (options.AllowNotFoundRecovery && IsStatus(err, 404))
			var transport *TransportError
			if errors.As(err, &transport) && safeMethod(transport.Method) {
				retryable = true
			}
			visibilityDelay := false
			if options.AllowTaskVisibilityDelay {
				var rf *Error
				if errors.As(err, &rf) && rf.StatusCode == http.StatusInternalServerError {
					var body struct {
						Code int `json:"code"`
					}
					visibilityDelay = json.Unmarshal(rf.Body, &body) == nil && body.Code == ieitTaskVisibilityDelayCode
				}
			}
			if visibilityDelay || (retryable && failures < options.MaxTransientFailures) {
				failures++
				if err = waitContext(ctx, interval); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		failures = 0
		result := &OperationResult{Response: response}
		if validJSON(response.Body) {
			var task models.Task
			if err = json.Unmarshal(response.Body, &task); err != nil {
				return nil, err
			}
			result.Task = &task
			if err = taskFailure(&task); err != nil {
				return result, err
			}
			if Value(task.TaskState) == TaskCompleted {
				return result, nil
			}
			if !monitor && Value(task.TaskState) == "" {
				return result, errors.New("task resource is missing TaskState")
			}
		}
		if monitor && response.StatusCode != http.StatusAccepted && (result.Task == nil || Value(result.Task.TaskState) == "") {
			return result, nil
		}
		if !monitor && result.Task == nil {
			return result, errors.New("task resource response is empty")
		}
		delay := interval
		if response.Header.Get(HeaderRetryAfter) != "" {
			delay = retryDelay(response.Header, interval, 0)
		}
		if err = waitContext(ctx, delay); err != nil {
			return result, fmt.Errorf("waiting for Redfish task: %w", err)
		}
	}
}
