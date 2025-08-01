package taskmaster

import "errors"

var (
	ErrTaskUnknown        = errors.New("task unknown")
	ErrTaskAlreadyRunning = errors.New("task already running")
	ErrTaskNotRunning     = errors.New("task is not running")

	ServiceClosed = errors.New("service closed")
)
