package models

// Standard task states are shared by Client workflows and OEM resource implementations.
const (
	TaskNew            = "New"
	TaskRunning        = "Running"
	TaskPending        = "Pending"
	TaskCompleted      = "Completed"
	TaskException      = "Exception"
	TaskKilled         = "Killed"
	TaskCancelled      = "Cancelled"
	TaskInterrupted    = "Interrupted"
	TaskStatusCritical = "Critical"
	TaskStatusOK       = "OK"
)
