package main

import "errors"

var (
	TerminatedBySignal = errors.New("terminated by signal")
)
