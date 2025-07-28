package taskmaster

import "errors"

var (
	ErrTaskUnknow         = errors.New("task unknow")
	ErrTaskAlreadyRunning = errors.New("task already running")
)
