package errors

import (
	"github.com/rednote-infra/redfish-sdk-go/models"
	sdkresponse "github.com/rednote-infra/redfish-sdk-go/response"
)

// AsyncOperationError retains task and response context for a failed asynchronous operation.
type AsyncOperationError struct {
	Task     *models.Task
	Response *sdkresponse.Response
	Message  string
}

func (e *AsyncOperationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "asynchronous operation failed"
}

// TaskFailure reports terminal Redfish task failures without changing their established error type.
func TaskFailure(task *models.Task) error {
	state := models.Value(task.TaskState)
	switch state {
	case models.TaskException, models.TaskKilled, models.TaskCancelled, models.TaskInterrupted:
		return &AsyncOperationError{Task: task, Message: "Redfish task terminated with state " + state}
	}
	if state == models.TaskCompleted && models.Value(task.TaskStatus) == models.TaskStatusCritical {
		return &AsyncOperationError{Task: task, Message: "Redfish task completed with critical status"}
	}
	return nil
}
