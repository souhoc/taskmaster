package taskmaster

import "errors"

var (
	ErrProcessUnknown        = errors.New("process's unknown")
	ErrProcessAlreadyStarted = errors.New("process's already started")
	ErrProcessNil            = errors.New("process's nil")
	ErrProcessIsNotRunning   = errors.New("process is not running")

	ServiceClosed = errors.New("service closed")
)
